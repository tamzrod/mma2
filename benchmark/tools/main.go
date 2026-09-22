// MMA2 end-to-end Modbus TCP benchmark. Standard library only.
package main

import (
 "encoding/binary"
 "encoding/csv"
 "flag"
 "fmt"
 "math"
 "net"
 "os"
 "path/filepath"
 "runtime"
 "sort"
 "strconv"
 "sync"
 "sync/atomic"
 "time"
)

type sample struct { latency time.Duration; err error }
func main() {
 addr:=flag.String("addr","127.0.0.1:15020","isolated MMA2 Modbus TCP listener")
 unit:=flag.Int("unit",1,"configured Modbus unit ID")
 clients:=flag.Int("clients",1,"parallel persistent connections")
 duration:=flag.Duration("duration",10*time.Second,"measurement duration")
 warmup:=flag.Duration("warmup",2*time.Second,"warmup duration")
 out:=flag.String("out","benchmark/results/local","output directory")
 flag.Parse()
 if *clients<1 || *unit<1 || *unit>247 || *duration<=0 || *warmup<0 { panic("invalid arguments") }
 if err:=os.MkdirAll(*out,0755);err!=nil {panic(err)}
 fmt.Println("MMA2 Modbus TCP FC3 holding register 0 quantity 1; verify benchmark target is isolated")
 run(*addr,byte(*unit),*clients,*warmup,false,nil)
 results:=make(chan sample,8192)
 start:=time.Now()
 run(*addr,byte(*unit),*clients,*duration,true,results)
 elapsed:=time.Since(start)
 close(results)
 var lat []float64; errors:=0
 for s:=range results {if s.err!=nil {errors++;continue};lat=append(lat,float64(s.lat.Microseconds())/1000)}
 sort.Float64s(lat)
 pct:=func(p float64)float64 {if len(lat)==0{return math.NaN()};return lat[int(math.Ceil(p*float64(len(lat))))-1]}
 f,err:=os.Create(filepath.Join(*out,"summary.csv"));if err!=nil {panic(err)}
 w:=csv.NewWriter(f)
 _=w.Write([]string{"target","clients","duration_seconds","successes","errors","ops_per_second","p50_ms","p95_ms","p99_ms","go_version"})
 _=w.Write([]string{*addr,strconv.Itoa(*clients),fmt.Sprintf("%.6f",elapsed.Seconds()),strconv.Itoa(len(lat)),strconv.Itoa(errors),fmt.Sprintf("%.3f",float64(len(lat))/elapsed.Seconds()),fmt.Sprintf("%.3f",pct(.50)),fmt.Sprintf("%.3f",pct(.95)),fmt.Sprintf("%.3f",pct(.99)),runtime.Version()})
 w.Flush();if err:=w.Error();err!=nil {panic(err)};if err:=f.Close();err!=nil {panic(err)}
 raw,err:=os.Create(filepath.Join(*out,"latency_ms.csv"));if err!=nil {panic(err)}
 rw:=csv.NewWriter(raw);_=rw.Write([]string{"latency_ms"});for _,v:=range lat{_=rw.Write([]string{fmt.Sprintf("%.3f",v)})};rw.Flush();if err:=rw.Error();err!=nil {panic(err)};_=raw.Close()
 svg:=fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="640" height="260"><rect width="640" height="260" fill="white"/><text x="20" y="30" font-size="20">MMA2 Modbus TCP latency (ms)</text><text x="20" y="70">p50: %.3f ms</text><text x="20" y="105">p95: %.3f ms</text><text x="20" y="140">p99: %.3f ms</text><text x="20" y="190">Successful requests: %d | errors: %d</text><text x="20" y="225">Throughput: %.1f ops/s</text></svg>`,pct(.5),pct(.95),pct(.99),len(lat),errors,float64(len(lat))/elapsed.Seconds())
 if err:=os.WriteFile(filepath.Join(*out,"summary.svg"),[]byte(svg),0644);err!=nil{panic(err)}
 fmt.Printf("success=%d errors=%d ops/s=%.1f p50=%.3fms p95=%.3fms p99=%.3fms\n",len(lat),errors,float64(len(lat))/elapsed.Seconds(),pct(.5),pct(.95),pct(.99))
 if errors>0 || len(lat)==0 {os.Exit(1)}
}
func run(addr string,unit byte,clients int,d time.Duration,record bool,results chan<- sample){
 var wg sync.WaitGroup;var seq atomic.Uint32;deadline:=time.Now().Add(d)
 for i:=0;i<clients;i++ {wg.Add(1);go func(){defer wg.Done();var conn net.Conn;defer func(){if conn!=nil{_=conn.Close()}}();for time.Now().Before(deadline){if conn==nil{c,e:=net.DialTimeout("tcp",addr,time.Second);if e!=nil{if record{results<-sample{err:e}};time.Sleep(10*time.Millisecond);continue};conn=c};id:=uint16(seq.Add(1));request:=[]byte{0,0,0,0,0,6,unit,3,0,0,0,1};binary.BigEndian.PutUint16(request[:2],id);_ = conn.SetDeadline(time.Now().Add(time.Second));start:=time.Now();_,e:=conn.Write(request);if e==nil{head:=make([]byte,7);_,e=readFull(conn,head);if e==nil{n:=int(binary.BigEndian.Uint16(head[4:6]))-1;if n<2||n>253||binary.BigEndian.Uint16(head[:2])!=id||head[6]!=unit{e=fmt.Errorf("invalid MBAP response")}else{body:=make([]byte,n);_,e=readFull(conn,body);if e==nil&&(n!=4||body[0]!=3||body[1]!=2){e=fmt.Errorf("unexpected FC3 response")}}}};if record{results<-sample{latency:time.Since(start),err:e}};if e!=nil{_=conn.Close();conn=nil}}}()};wg.Wait()
}
func readFull(c net.Conn,b []byte)(int,error){n:=0;for n<len(b){k,e:=c.Read(b[n:]);n+=k;if e!=nil{return n,e}};return n,nil}
