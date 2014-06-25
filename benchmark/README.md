# Cider Benchmark #

This directory contains an executable implementing a simple Cider benchmark.
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

## Results ##

### Machine A ###

MacBook Pro, 2.7 GHz Intel Core i7, 4 GB RAM, disk 5400 rpm:

### Machine B ###

iMac, 3.1 GHz Intel Core i5, 4 CPUs, 12 GB 1333MHz DDR3 RAM, disk 7200 rpm, running Mac OS X 10.8.5

#### Single OS Thread ####

```
$ godep go run main.go -mode=noop
Benchmark mode: noop
Using 1 thread(s)
Starting a benchmark round, N=1
Starting a benchmark round, N=100
Starting a benchmark round, N=10000
      10000	    102475 ns/op
Total duration: 1.024754911s
```

In other words, **10 000 builds/s**.

```
$ godep go run main.go -mode=discard
Benchmark mode: discard
Using 1 thread(s)
Starting a benchmark round, N=1
Starting a benchmark round, N=10
Starting a benchmark round, N=20
      20	  88243379 ns/op
Total duration: 1.76486759s
```

In other words, **11 builds/s** (88 ms/build).

```
$ godep go run main.go -mode=streaming
Benchmark mode: streaming
Using 1 thread(s)
Starting a benchmark round, N=1
Starting a benchmark round, N=10
      10	 116437369 ns/op
Total duration: 1.164373695s
```

In other words, **8 builds/s** (116 ms/build).

#### Multiple OS Threads ####

```
$ godep go run main.go -threads=4 -mode=noop
Benchmark mode: noop
Using 4 thread(s)
Starting a benchmark round, N=1
Starting a benchmark round, N=100
Starting a benchmark round, N=10000
Starting a benchmark round, N=50000
   50000	     47613 ns/op
Total duration: 2.380661861s
```

In other words, **21 000 builds/s**.

```
$ godep go run main.go -threads=4 -mode=discard
Benchmark mode: discard
Using 4 thread(s)
Starting a benchmark round, N=1
Starting a benchmark round, N=10
Starting a benchmark round, N=50
      50	  59514214 ns/op
Total duration: 2.975710744s
```

In other words, **17 builds/s** (59 ms/build).

```
$ godep go run main.go -threads=4 -mode=streaming
Benchmark mode: streaming
Using 4 thread(s)
Starting a benchmark round, N=1
Starting a benchmark round, N=10
Starting a benchmark round, N=50
      50	  65710241 ns/op
Total duration: 3.285512054s
```

In other words, **15 builds/s** (65 ms/build).

## Conclusion ##

The communication framework itself imposes very little overhead compared to what
it takes to actually clone the repository and run the build script. In real
deployment Cider would much faster run out of network bandwidth or the
database where the build output is being saved would become a bottleneck.
