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
Starting a benchmark round, N=1
Starting a benchmark round, N=20
Starting a benchmark round, N=50
      50	  35462621 ns/op
Total duration: 1.773131095s
```

which is **35 ms/build**.
