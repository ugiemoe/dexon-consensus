package main

import (
	"flag"
	"log"
	"math"
	"math/rand"
	"os"
	"runtime"
	"runtime/pprof"
	"time"

	"github.com/dexon-foundation/dexon-consensus-core/core/test"
	integration "github.com/dexon-foundation/dexon-consensus-core/integration_test"
	"github.com/dexon-foundation/dexon-consensus-core/simulation/config"
)

var (
	configFile = flag.String("config", "", "path to simulation config file")
	cpuprofile = flag.String("cpuprofile", "", "write cpu profile to `file`")
	memprofile = flag.String("memprofile", "", "write memory profile to `file`")
)

func main() {
	flag.Parse()
	rand.Seed(time.Now().UnixNano())

	if *configFile == "" {
		log.Fatal("error: no configuration file specified")
	}
	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			log.Fatal("could not create CPU profile: ", err)
		}
		if err := pprof.StartCPUProfile(f); err != nil {
			log.Fatal("could not start CPU profile: ", err)
		}
		defer pprof.StopCPUProfile()
	}

	cfg, err := config.Read(*configFile)
	if err != nil {
		log.Fatal("unable to read config: ", err)
	}
	// Setup latencies, nodes.
	networkLatency := &test.NormalLatencyModel{
		Sigma: cfg.Networking.Sigma,
		Mean:  cfg.Networking.Mean,
	}
	proposingLatency := &test.NormalLatencyModel{
		Sigma: cfg.Node.Legacy.ProposeIntervalSigma,
		Mean:  cfg.Node.Legacy.ProposeIntervalMean,
	}
	// Setup nodes and other consensus related stuffs.
	apps, dbs, nodes, err := integration.PrepareNodes(
		cfg.Node.Num, networkLatency, proposingLatency)
	if err != nil {
		log.Fatal("could not setup nodes: ", err)
	}
	blockPerNode := int(math.Ceil(
		float64(cfg.Node.MaxBlock) / float64(cfg.Node.Num)))
	sch := test.NewScheduler(
		test.NewStopByConfirmedBlocks(blockPerNode, apps, dbs))
	for nID, v := range nodes {
		sch.RegisterEventHandler(nID, v)
		if err = sch.Seed(integration.NewProposeBlockEvent(
			nID, time.Now().UTC())); err != nil {

			log.Fatal("unable to set seed simulation events: ", err)
		}
	}
	// Run the simulation.
	sch.Run(cfg.Scheduler.WorkerNum)
	if err = integration.VerifyApps(apps); err != nil {
		log.Fatal("consensus result is not incorrect: ", err)
	}
	// Prepare statistics.
	stats, err := integration.NewStats(sch.CloneExecutionHistory(), apps)
	if err != nil {
		log.Fatal("could not generate statistics: ", err)
	}
	if *memprofile != "" {
		f, err := os.Create(*memprofile)
		if err != nil {
			log.Fatal("could not create memory profile: ", err)
		}
		runtime.GC() // get up-to-date statistics
		if err := pprof.WriteHeapProfile(f); err != nil {
			log.Fatal("could not write memory profile: ", err)
		}
		f.Close()
	}

	log.Printf("BPS: %v\n", stats.BPS)
	log.Printf("ExecutionTime: %v\n", stats.ExecutionTime)
	log.Printf("Prepare: %v\n", time.Duration(stats.All.PrepareExecLatency))
	log.Printf("Process: %v\n", time.Duration(stats.All.ProcessExecLatency))
}
