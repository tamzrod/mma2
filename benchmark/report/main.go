// Command report converts benchmark/tools CSV outputs into a one-page PDF.
// No third-party dependencies. Run: go run ./benchmark/report -input benchmark/results/local -output benchmark/results/local/report.pdf
package main

import (
 "bytes"
 "encoding/csv"
 "flag"
 "fmt"
 "math"
 "os"
 "path/filepath"
 "strconv"
 "strings"
)

func main() {
 input:=flag.String("input","benchmark/results/local","directory containing summary.csv and latency_ms.csv")
 output:=flag.String("output","benchmark/results/local/report.pdf","PDF output path")
 flag.Parse()
 rows,err:=read(filepath.Join(*input,"summary.csv"));must(err)
 if len(rows)!=2 {panic("summary.csv must contain exactly one header and one result row")}
 values:=map[string]string{};for i,k:=range rows[0] {if i<len(rows[1]) {values[k]=rows[1][i]}}
 lat,err:=read(filepath.Join(*input,"latency_ms.csv"));must(err)
 if len(lat)<2 || lat[0][0]!="latency_ms" {panic("latency_ms.csv contains no latency samples")}
 var samples []float64
 for _,r:=range lat[1:] {if len(r)==0 {continue};v,e:=strconv.ParseFloat(r[0],64);must(e);if math.IsNaN(v)||math.IsInf(v,0)||v<0 {panic("invalid latency")};samples=append(samples,v)}
 if len(samples)==0 {panic("no latency samples")}
 var content strings.Builder
 content.WriteString("0.12 0.19 0.31 rg 0 740 612 52 re f\n1 1 1 rg BT /F1 20 Tf 38 759 Td (MMA2 Benchmark Report) Tj ET\n0 0 0 rg\n")
 text:=func(x,y int,s string){fmt.Fprintf(&content,"BT /F1 11 Tf %d %d Td (%s) Tj ET\n",x,y,escape(s))}
 text(38,713,"Measured Modbus TCP FC3 appliance results")
 fields:=[]struct{label,key string}{{"Target","target"},{"Clients","clients"},{"Duration (s)","duration_seconds"},{"Successes","successes"},{"Errors","errors"},{"Throughput (ops/s)","ops_per_second"},{"p50 (ms)","p50_ms"},{"p95 (ms)","p95_ms"},{"p99 (ms)","p99_ms"},{"Go version","go_version"}}
 for i,f:=range fields{text(38,685-i*20,f.label+": "+values[f.key])}
 text(38,459,"Latency distribution (successful requests; histogram in milliseconds)")
 const left,bottom,width,height=55.,115.,510.,315.
 fmt.Fprintf(&content,"0 0 0 RG %.1f %.1f m %.1f %.1f l %.1f %.1f l S\n",left,bottom+height,left,bottom,left+width,bottom)
 max:=0.;for _,v:=range samples{if v>max{max=v}};if max==0{max=1}
 const bins=30
 counts:=make([]int,bins);peak:=0
 for _,v:=range samples{i:=int(v/max*bins);if i>=bins{i=bins-1};counts[i]++;if counts[i]>peak{peak=counts[i]}}
 for i,n:=range counts{barH:=float64(n)/float64(peak)*(height-15);x:=left+float64(i)*width/bins;fmt.Fprintf(&content,"0.18 0.44 0.66 rg %.2f %.2f %.2f %.2f re f\n",x,bottom,width/bins-1,barH)}
 text(55,95,"0 ms");text(450,95,fmt.Sprintf("max %.3f ms",max))
 text(38,60,"Generated from recorded CSV data. No inferred CPU/RAM or RBE measurements.")
 pdf:=buildPDF(content.String())
 must(os.MkdirAll(filepath.Dir(*output),0755));must(os.WriteFile(*output,pdf,0644))
 fmt.Printf("PDF generated: %s (%d latency samples)\n",*output,len(samples))
}
func read(path string)([][]string,error){f,e:=os.Open(path);if e!=nil{return nil,e};defer f.Close();return csv.NewReader(f).ReadAll()}
func must(e error){if e!=nil{panic(e)}}
func escape(s string)string {s=strings.ReplaceAll(s,"\\","\\\\");s=strings.ReplaceAll(s,"(","\\(");return strings.ReplaceAll(s,")","\\)")}
func buildPDF(stream string)[]byte{
 objects:=[]string{
 "<< /Type /Catalog /Pages 2 0 R >>",
 "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
 "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>",
 "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
 fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream",len(stream),stream),
 }
 var b bytes.Buffer;b.WriteString("%PDF-1.4\n")
 offsets:=[]int{0};for i,obj:=range objects{offsets=append(offsets,b.Len());fmt.Fprintf(&b,"%d 0 obj\n%s\nendobj\n",i+1,obj)}
 xref:=b.Len();fmt.Fprintf(&b,"xref\n0 %d\n0000000000 65535 f \n",len(offsets));for _,o:=range offsets[1:]{fmt.Fprintf(&b,"%010d 00000 n \n",o)}
 fmt.Fprintf(&b,"trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n",len(offsets),xref)
 return b.Bytes()
}
