package main

import (
	"flag"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/paprikaci/paprika/build"
	"github.com/paprikaci/paprika/data"
	"github.com/paprikaci/paprika/master"
	"github.com/paprikaci/paprika/slave"

	"github.com/cihub/seelog"
)

const (
	listenAddress  = "localhost:8910"
	connectAddress = "ws://" + listenAddress + "/connect"
	token          = "bublifuk"

	modeNoop      = "noop"
	modeDiscard   = "discard"
	modeStreaming = "streaming"
)

var (
	numThreads int    = 1
	mode       string = "streaming"
)

func main() {
	log.SetFlags(0)
	seelog.ReplaceLogger(seelog.Disabled)

	flag.IntVar(&numThreads, "threads", numThreads, "number of OS threads to use")
	flag.StringVar(&mode, "mode", mode, "benchmark mode; can be 'noop', 'discard' or 'streaming'")
	flag.Parse()

	switch mode {
	case modeNoop:
	case modeDiscard:
	case modeStreaming:
	default:
		log.Fatalf("unknown benchmark mode: %v", mode)
	}
	log.Printf("Benchmark mode: %v\n", mode)

	if numThreads < 1 {
		log.Fatalf("invalid -threads value: %v", numThreads)
	}
	log.Printf("Using %v thread(s)\n", numThreads)
	runtime.GOMAXPROCS(numThreads)

	res := testing.Benchmark(benchmark)
	log.Println(res)
	log.Printf("Total duration: %v\n", res.T)
}

func benchmark(b *testing.B) {
	const prefix = "paprika-benchmark"

	log.Printf("Starting a benchmark round, N=%v\n", b.N)

	// Create a temporary project directory to be cloned.
	repository, err := ioutil.TempDir("", prefix)
	if err != nil {
		log.Fatal(err)
	}
	if mode != modeNoop {
		if err := initRepository(b, repository); err != nil {
			log.Fatal(err)
		}
		defer nuke(repository)
	}

	// Create a temporary workspace for the build slave.
	workspace, err := ioutil.TempDir("", prefix)
	if err != nil {
		log.Fatal(err)
	}
	defer nuke(workspace)

	// Run the build master.
	buildMaster := master.New(listenAddress, token)
	buildMaster.Listen()
	defer buildMaster.Terminate()

	// Run the build slave.
	buildSlave := slave.New("Pepa", workspace, uint(runtime.NumCPU()))
	go buildSlave.Connect(connectAddress, token)
	defer buildSlave.Terminate()

	// Wait a bit for the things to bind.
	time.Sleep(time.Second)
	// Check here if there are already any errors.
	select {
	case <-buildMaster.Terminated():
		log.Fatal(buildMaster.Wait())
	case <-buildSlave.Terminated():
		log.Fatal(buildSlave.Wait())
	default:
	}

	// Prepare the build args.
	client, err := build.Dial(connectAddress, token)
	if err != nil {
		log.Fatal(err)
	}
	repositoryBaseURL := "git+file://" + repository
	args := make([]*data.BuildArgs, b.N)
	for i := 0; i < b.N; i++ {
		args[i] = &data.BuildArgs{
			Repository: repositoryBaseURL + "#b" + strconv.Itoa(i),
			Script:     "build.sh",
		}
		if mode == modeNoop {
			args[i].Noop = true
		}
	}

	// Prepare the build requests.
	requests := make([]*build.BuildRequest, b.N)
	for i := 0; i < b.N; i++ {
		requests[i] = client.NewBuildRequest("paprika.any.bash", args[i])
		if mode == modeStreaming {
			// This makes Paprika stream the output, but discard it.
			requests[i].Stdout = ioutil.Discard
		}
		requests[i].Stderr = os.Stderr
	}

	// Fire all the requests at once, awwwww.
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		requests[i].GoExecute()
	}

	// Wait for all the requests.
	for i := 0; i < b.N; i++ {
		result, err := requests[i].Wait()
		if err != nil {
			log.Println(err)
		}
		if result.Error != "" {
			log.Println(result.Error)
		}
	}
	b.StopTimer()
}

func nuke(path string) {
	if err := os.RemoveAll(path); err != nil {
		log.Println(err)
	}
}

func initRepository(b *testing.B, path string) error {
	cmd := exec.Command("git", "init")
	cmd.Dir = path
	if err := cmd.Run(); err != nil {
		return err
	}

	src, err := os.Open("data/build.sh")
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := os.Create(filepath.Join(path, "build.sh"))
	if err != nil {
		return err
	}
	defer dst.Close()
	_, err = io.Copy(dst, src)
	if err != nil {
		return err
	}

	cmd = exec.Command("git", "add", "build.sh")
	cmd.Dir = path
	if err := cmd.Run(); err != nil {
		return err
	}

	cmd = exec.Command("git", "commit", "-m", "Commit the build script")
	cmd.Dir = path
	if err := cmd.Run(); err != nil {
		return err
	}

	for i := 0; i < b.N; i++ {
		cmd = exec.Command("git", "branch", "b"+strconv.Itoa(i))
		cmd.Dir = path
		if err := cmd.Run(); err != nil {
			return err
		}
	}
	return nil
}
