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
   `data/build.sh` in a standalone `bash` process.

## Results ##

Using MacBook Pro, 2.7 GHz Intel Core i7, 4 GB RAM, disk 5400 rpm:

```
$ go run main.go
Using 1 thread(s)
Starting a benchmark round, N=1
Starting a benchmark round, N=20
Starting a benchmark round, N=50
      50	  35462621 ns/op
Total duration: 1.773131095s
```

which is **35 ms/build**, or **28 builds/s**.

Setting `GOMAXPROCS` to more than `1` does not make any apparent difference.

### NOOP Builds ###

Now setting the build slave to perform NOOP builds yields the following results.
That actually means that only the communication framework is being measured.

```
$ go run main.go -noop
NOOP builds enabled
Using 1 thread(s)
Starting a benchmark round, N=1
Starting a benchmark round, N=100
Starting a benchmark round, N=10000
   10000	    104065 ns/op
Total duration: 1.040658241s
```

which is **10 000 empty builds/s**.

### TODO ###

* Try with builds that are generating a lot of live output.

## Conclusion ##

The communication framework itself imposes very little overhead compared to what
it takes to actually clone the repository and run the build script. In real
deployment Paprika would much faster run out of network bandwidth or the
database where the builds output is being saved would become a bottleneck.
