package main

import (
	"bytes"
	"flag"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/cider/cider/build"
	"github.com/cider/cider/data"
	"github.com/cider/cider/master"
	"github.com/cider/cider/slave"

	"github.com/cihub/seelog"
	"github.com/garyburd/redigo/redis"
)

const (
	listenAddress  = "localhost:8910"
	connectAddress = "ws://" + listenAddress + "/connect"
	token          = "bublifuk"

	modeNoop      = "noop"
	modeDiscard   = "discard"
	modeStreaming = "streaming"
	modeRedis     = "redis"
)

var (
	numThreads int    = 1
	mode       string = "streaming"
	redisAddr  string = "localhost:6379"
)

func main() {
	log.SetFlags(0)
	seelog.ReplaceLogger(seelog.Disabled)

	flag.IntVar(&numThreads, "threads", numThreads, "number of OS threads to use")
	flag.StringVar(&mode, "mode", mode, "benchmark mode; (noop|discard|streaming|redis)")
	flag.StringVar(&redisAddr, "redis_addr", redisAddr, "Redis address")
	flag.Parse()

	switch mode {
	case modeNoop:
	case modeDiscard:
	case modeStreaming:
	case modeRedis:
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
	const prefix = "cider-benchmark"

	log.Printf("Starting a benchmark round, N=%v\n", b.N)

	// Set up Redis if necessary.
	var (
		pool *redis.Pool
		err  error
	)
	if mode == modeRedis {
		pool, err = initRedis(b)
		if err != nil {
			log.Fatal(err)
		}
		defer pool.Close()
	}

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
	// Check for errors here already, just in case.
	select {
	case <-buildMaster.Terminated():
		log.Fatal(buildMaster.Wait())
	case <-buildSlave.Terminated():
		log.Fatal(buildSlave.Wait())
	default:
	}

	// Connect the build client to the build master.
	client, err := build.Dial(connectAddress, token)
	if err != nil {
		log.Fatal(err)
	}

	// Run b.N goroutines, each firing one request.
	var wg sync.WaitGroup
	wg.Add(b.N)
	b.ResetTimer()
	repositoryBaseURL := "git+file://" + repository
	for i := 0; i < b.N; i++ {
		go func(index int) {
			defer wg.Done()
			args := &data.BuildArgs{
				Repository: repositoryBaseURL + "#b" + strconv.Itoa(index),
				Script:     "build.sh",
			}
			req := client.NewBuildRequest("cider.any.bash", args)
			req.Stderr = os.Stderr

			var stdout io.Writer
			switch mode {
			case modeNoop:
				args.Noop = true
			case modeStreaming:
				// This makes Cider stream the output, but then it is discarded
				// by the client library since the writer points to /dev/null.
				stdout = ioutil.Discard
			case modeRedis:
				stdout = new(bytes.Buffer)
			}
			req.Stdout = stdout

			res, err := req.Execute()
			if err != nil {
				log.Println(err)
				return
			}
			if res.Error != "" {
				log.Println(res.Error)
			}

			if mode == modeRedis {
				conn := pool.Get()
				defer conn.Close()
				_, err := conn.Do("SET", index, stdout.(*bytes.Buffer).Bytes())
				if err != nil {
					log.Println(err)
				}
			}
		}(i)
	}

	wg.Wait()
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

	if err := writeBuildScript(filepath.Join(path, "build.sh")); err != nil {
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

func initRedis(b *testing.B) (*redis.Pool, error) {
	pool := redis.NewPool(func() (redis.Conn, error) {
		return redis.Dial("tcp", redisAddr)
	}, b.N)

	for i := 0; i < b.N; i++ {
		conn := pool.Get()
		defer conn.Close()
		if _, err := conn.Do("SET", i, 0); err != nil {
			return nil, err
		}
	}

	return pool, nil
}

func writeBuildScript(path string) error {
	dst, err := os.Create(path)
	if err != nil {
		return err
	}
	defer dst.Close()

	if _, err := io.WriteString(dst, "for i in $(seq 10); do cat <<-EOF\n"); err != nil {
		return err
	}

	for i := 0; i < 1000; i++ {
		if _, err := io.WriteString(dst, "BYL JSEM TU! FANTOMAS\n"); err != nil {
			return err
		}
	}

	if _, err := io.WriteString(dst, "EOF\ndone\n"); err != nil {
		return err
	}
	return nil
}
