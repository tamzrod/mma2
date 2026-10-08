package config

import (
    "fmt"
    "path/filepath"
    "strings"

    "mma2/internal/memorycore"
)

// MemoryPersistenceConfig belongs exclusively to listeners[].memory[].
// When enabled, an omitted range list persists every allocated area.
type MemoryPersistenceConfig struct {
    Enabled bool `yaml:"enabled"`
    Directory string `yaml:"directory"`
    Ranges *MemoryPersistenceRanges `yaml:"ranges"`
}

// Each area accepts multiple non-overlapping ranges without a segments wrapper.
type MemoryPersistenceRanges struct {
    Coils []PersistenceArea `yaml:"coils"`
    DiscreteInputs []PersistenceArea `yaml:"discrete_inputs"`
    HoldingRegs []PersistenceArea `yaml:"holding_registers"`
    InputRegs []PersistenceArea `yaml:"input_registers"`
}

func memorySelections(r *MemoryPersistenceRanges) []ResolvedPersistenceArea {
    if r == nil { return nil }
    var out []ResolvedPersistenceArea
    add := func(a memorycore.Area, ranges []PersistenceArea) {
        for _, x := range ranges {
            out = append(out, ResolvedPersistenceArea{Area:a, Start:x.Start, Count:x.Count})
        }
    }
    add(memorycore.AreaCoils,r.Coils)
    add(memorycore.AreaDiscreteInputs,r.DiscreteInputs)
    add(memorycore.AreaHoldingRegs,r.HoldingRegs)
    add(memorycore.AreaInputRegs,r.InputRegs)
    return out
}

// BuildPerMemoryPersistencePlans validates and isolates each enabled memory.
// Each plan owns exactly one (Port, UnitID); no global persistence plan exists.
func BuildPerMemoryPersistencePlans(cfg *Config) (map[memorycore.MemoryID]*ResolvedPersistence,error) {
    if cfg == nil { return nil,fmt.Errorf("config is nil") }
    if cfg.Persistence != nil {
        return nil, fmt.Errorf("root-level persistence is unsupported; configure listeners[].memory[].persistence")
    }
    result:=make(map[memorycore.MemoryID]*ResolvedPersistence)
    directories:=make(map[string]memorycore.MemoryID)
    for li,l := range cfg.Ingress {
        if len(l.Memory)==0 {continue}
        port,err:=parseListenPort(l.Listen)
        if err != nil { return nil,fmt.Errorf("listeners[%d]: %w",li,err) }
        for mi,mem := range l.Memory {
            p:=mem.Persistence
            if p==nil || !p.Enabled { continue }
            path:=fmt.Sprintf("listeners[%d].memory[%d].persistence",li,mi)
            if strings.TrimSpace(p.Directory)=="" { return nil,fmt.Errorf("%s.directory is required",path) }
            dir,err:=filepath.Abs(filepath.Clean(p.Directory))
            if err!=nil {return nil,fmt.Errorf("%s.directory: %w",path,err)}
            id:=memorycore.MemoryID{Port:port,UnitID:mem.UnitID}
            if prev,ok:=directories[dir];ok {
                return nil,fmt.Errorf("%s.directory conflicts with identity (port=%d unit=%d); use distinct directories",path,prev.Port,prev.UnitID)
            }
            directories[dir]=id
            if _,ok:=result[id];ok {return nil,fmt.Errorf("%s: duplicate memory identity",path)}
            selected:=memorySelections(p.Ranges)
            if p.Ranges!=nil && len(selected)==0 {return nil,fmt.Errorf("%s.ranges cannot be empty",path)}
            allocations:=areaAllocations(mem)
            seen:=make(map[memorycore.Area][]PersistenceArea)
            for _,s:=range selected {
                if s.Count==0 {return nil,fmt.Errorf("%s: persistence range count must be > 0",path)}
                alloc,ok:=allocations[s.Area]
                if !ok || uint32(s.Start)<uint32(alloc.Start) || uint32(s.Start)+uint32(s.Count)>uint32(alloc.Start)+uint32(alloc.Count) {
                    return nil,fmt.Errorf("%s: range for %s is outside allocated memory",path,s.Area)
                }
                for _,prev:=range seen[s.Area] {
                    a,b:=uint32(s.Start),uint32(s.Start)+uint32(s.Count)
                    x,y:=uint32(prev.Start),uint32(prev.Start)+uint32(prev.Count)
                    if a<y && x<b {return nil,fmt.Errorf("%s: overlapping ranges for %s",path,s.Area)}
                }
                seen[s.Area]=append(seen[s.Area],PersistenceArea{Start:s.Start,Count:s.Count})
            }
            result[id]=&ResolvedPersistence{
                Directory:dir, RangesEmpty:p.Ranges==nil,
                Ranges:map[memorycore.MemoryID][]ResolvedPersistenceArea{id:selected},
            }
        }
    }
    return result,nil
}
