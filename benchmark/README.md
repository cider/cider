# Paprika Benchmark #

This directory contains an executable implementing a simple Paprika benchmark.
The benchmark is implemented using the standard Go
[testing](http://golang.org/pkg/testing/) package. If you don't know what `b.N`
is, this is the place where to find it.

The whole benchmark goes through the following steps:

1. Create a testing Git repository with a sample build script (see `data/build.sh`).
   `b.N` branches are created in that repository so that all `b.N` builds can run in
   parallel since the workspaces are different.
2. Start a build master and bind it to a local address.
3. Start a build slave node and connect it to the master. This slave is set up
   to use `runtime.NumCPU()` executors.
4. Fire `b.N` build jobs and wait for them to finish. Every job runs
   `data/build.sh` in a standalone `bash` process. This script prints a string
   to stdout using `echo` 10 000 times.

The benchmark can be run in 3 modes, which affects what happens during the build:

* `noop` - do not clone the repository, do not run any script, just return success
* `discard` - clone the repository, run the script, but do not stream the output back
* `streaming` - clone the repository, run the script and stream the output back
* `redis` - clone the repository, run the script, buffer the output, then save
  it into Redis (see `-redis_addr` to configure this mode)
* `files` - clone the repository, run the script, buffer the output, then save
  in into a file on the disk

## Results ##

### Machine A ###

MacBook Pro, 4 CPUs, 2.7 GHz Intel Core i7, 4 GB RAM (1333 DDR3), disk 5400 rpm, running Mac OS X 10.7.5.

#### Single OS Thread ####

```
$ for i in $(seq 5); do ./benchmark -mode=noop; echo; done
Benchmark mode: noop
Using 1 thread(s)
Starting a benchmark round, N=1
Starting a benchmark round, N=100
Starting a benchmark round, N=10000
   10000	    116141 ns/op
Total duration: 1.161419287s

Starting a benchmark round, N=10000
   10000	    118318 ns/op
Total duration: 1.183188426s

Starting a benchmark round, N=10000
   10000	    117466 ns/op
Total duration: 1.174662656s

Starting a benchmark round, N=10000
   10000	    117672 ns/op
Total duration: 1.176727186s

Starting a benchmark round, N=10000
   10000	    119870 ns/op
Total duration: 1.198708542s
```

```
$ for i in $(seq 5); do ./benchmark -mode=discard; echo; done
Benchmark mode: discard
Using 1 thread(s)
Starting a benchmark round, N=1
Starting a benchmark round, N=10
Starting a benchmark round, N=50
      50	  66049025 ns/op
Total duration: 3.302451256s

Starting a benchmark round, N=20
      20	 124588994 ns/op
Total duration: 2.491779898s

Starting a benchmark round, N=50
      50	  72269828 ns/op
Total duration: 3.613491432s

Starting a benchmark round, N=20
      20	  83775819 ns/op
Total duration: 1.675516384s

Starting a benchmark round, N=10
      10	 134034625 ns/op
Total duration: 1.340346252s
```

```
$ for i in $(seq 5); do ./benchmark -mode=streaming; echo; done
Benchmark mode: streaming
Using 1 thread(s)
Starting a benchmark round, N=1
Starting a benchmark round, N=10
Starting a benchmark round, N=50
      50	  55566140 ns/op
Total duration: 2.778307018s

Starting a benchmark round, N=20
      20	  50753613 ns/op
Total duration: 1.015072262s

Starting a benchmark round, N=10
      10	 106343609 ns/op
Total duration: 1.063436099s

Starting a benchmark round, N=20
      20	  87325214 ns/op
Total duration: 1.746504298s

Starting a benchmark round, N=50
      50	  67271365 ns/op
Total duration: 3.363568284s
```

```
$ for i in $(seq 5); do ./benchmark -mode=redis; echo; done
Benchmark mode: redis
Using 1 thread(s)
Starting a benchmark round, N=1
Starting a benchmark round, N=10
Starting a benchmark round, N=50
      50	  91469290 ns/op
Total duration: 4.573464539s

Starting a benchmark round, N=50
      50	  89834287 ns/op
Total duration: 4.491714376s

Starting a benchmark round, N=50
      50	  74857127 ns/op
Total duration: 3.742856379s

Starting a benchmark round, N=50
      50	  61327816 ns/op
Total duration: 3.066390801s

Starting a benchmark round, N=50
      50	  56668844 ns/op
Total duration: 2.833442247s
```

```
$ for i in $(seq 5); do ./benchmark -mode=files; echo; done
Benchmark mode: files
Using 1 thread(s)
Starting a benchmark round, N=1
Starting a benchmark round, N=10
      10	 105338329 ns/op
Total duration: 1.053383292s

Starting a benchmark round, N=10
      10	 118421312 ns/op
Total duration: 1.184213129s

Starting a benchmark round, N=50
      50	  59483523 ns/op
Total duration: 2.974176176s

Starting a benchmark round, N=50
      50	  64875806 ns/op
Total duration: 3.243790346s

Starting a benchmark round, N=10
      10	 120911117 ns/op
Total duration: 1.209111176s
```

#### Multiple OS Threads ####

```
$ for i in $(seq 5); do ./benchmark -mode=noop -threads=4; echo; done
Benchmark mode: noop
Using 4 thread(s)
Starting a benchmark round, N=1
Starting a benchmark round, N=100
Starting a benchmark round, N=10000
Starting a benchmark round, N=20000
   20000	     73202 ns/op
Total duration: 1.464042831s

Starting a benchmark round, N=20000
   20000	     71841 ns/op
Total duration: 1.43682846s

Starting a benchmark round, N=20000
   20000	     72978 ns/op
Total duration: 1.459565783s

Starting a benchmark round, N=20000
   20000	     72959 ns/op
Total duration: 1.4591914s

Starting a benchmark round, N=20000
   20000	     73456 ns/op
Total duration: 1.469125557s
```

```
$ for i in $(seq 5); do ./benchmark -mode=discard -threads=4; echo; done
Benchmark mode: discard
Using 4 thread(s)
Starting a benchmark round, N=1
Starting a benchmark round, N=10
Starting a benchmark round, N=50
      50	  60953392 ns/op
Total duration: 3.047669632s

Using 4 thread(s)
Starting a benchmark round, N=50
      50	  74873721 ns/op
Total duration: 3.743686079s

Benchmark mode: discard
Using 4 thread(s)
Starting a benchmark round, N=50
      50	  80092564 ns/op
Total duration: 4.004628243s

Benchmark mode: discard
Using 4 thread(s)
Starting a benchmark round, N=20
      20	  68473920 ns/op
Total duration: 1.369478414s

Benchmark mode: discard
Using 4 thread(s)
Starting a benchmark round, N=20
      20	  61124403 ns/op
Total duration: 1.222488079s
```

```
$ for i in $(seq 5); do ./benchmark -mode=streaming -threads=4; echo; done
Benchmark mode: streaming
Using 4 thread(s)
Starting a benchmark round, N=1
Starting a benchmark round, N=10
Starting a benchmark round, N=20
      20	  89590665 ns/op
Total duration: 1.791813309s

Benchmark mode: streaming
Using 4 thread(s)
Starting a benchmark round, N=50
      50	  62025025 ns/op
Total duration: 3.101251279s

Benchmark mode: streaming
Using 4 thread(s)
Starting a benchmark round, N=50
      50	  65817256 ns/op
Total duration: 3.290862834s

Benchmark mode: streaming
Using 4 thread(s)
Starting a benchmark round, N=50
      50	  73712222 ns/op
Total duration: 3.68561113s

Benchmark mode: streaming
Using 4 thread(s)
Starting a benchmark round, N=20
      20	  90325548 ns/op
Total duration: 1.806510968s
```

```
$ for i in $(seq 5); do ./benchmark -mode=redis -threads=4; echo; done
Benchmark mode: redis
Using 4 thread(s)
Starting a benchmark round, N=1
Starting a benchmark round, N=10
Starting a benchmark round, N=50
      50	  67857096 ns/op
Total duration: 3.392854823s

Starting a benchmark round, N=50
      50	  69941030 ns/op
Total duration: 3.497051502s

Starting a benchmark round, N=50
      50	  71235698 ns/op
Total duration: 3.561784913s

Starting a benchmark round, N=50
      50	  58204349 ns/op
Total duration: 2.910217469s

Starting a benchmark round, N=50
      50	  68735020 ns/op
Total duration: 3.436751014s
```

```
$ for i in $(seq 5); do ./benchmark -mode=files -threads=4; echo; done
Benchmark mode: files
Using 4 thread(s)
Starting a benchmark round, N=1
Starting a benchmark round, N=10
      10	 106588232 ns/op
Total duration: 1.065882329s

Starting a benchmark round, N=20
      20	  91778911 ns/op
Total duration: 1.83557823s

Starting a benchmark round, N=20
      20	  70745534 ns/op
Total duration: 1.414910694s

Starting a benchmark round, N=20
      20	  92100082 ns/op
Total duration: 1.842001646s

Starting a benchmark round, N=10
      10	 141744087 ns/op
Total duration: 1.41744087s
```

### Machine B ###

iMac, 3.1 GHz Intel Core i5, 4 CPUs, 12 GB RAM (1333MHz DDR3), disk 7200 rpm, running Mac OS X 10.8.5.

#### Single OS Thread ####

#### Multiple OS Threads ####

## Conclusion ##

The communication framework itself imposes very little overhead compared to what
it takes to actually clone the repository and run the build script. In real
deployment Paprika would much faster run out of network bandwidth or the
database where the builds output is being saved would become a bottleneck.

Running the benchmark using **4 threads instead of 1** makes the system able to run
up to **2 times more builds** per second.
