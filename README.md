# Cider CI Server Extender #

There are many cool Continuous Integration hosted services, like Travis CI,
Circle CI, Drone.io and many others. The problem is that these services mostly
support Linux builds only since that is the easiest thing to implement. The
reality is unfortunately not that beautiful and often there is a need for
Windows or Mac OS X builds.

Since [we](https://www.salsitasoft.com) really liked these hosted services, and
we also needed Windows and Mac OS X builds, we decided to create **Cider**,
which is something that could be called a CI server *extender*. The idea is simple.
You use your favourite hosted CI server, but in case you need a build environment
that is not supported, you use a command line utility to connect to another
CI system with its own set of build slaves. You trigger a build there, the
output is being steamed to the console and the output itself is saved in the
hosted CI server that you use. In other words, the build job is trigger in the
hosted CI server, but the task itself is executed on your own build slave.

To see how a Cider-compatible project repository looks like and how the output
is streamed back to the console, check the
[demo repository](https://github.com/cider/cider-example).

## cider Command

`cider` executable implements the whole Cider CI server functionality. The
functionality is split into subcommands:

* `cider master` starts a master node.
* `cider slave` starts a slave node.
* `cider build` connects to the chosen master node and triggers a build.

Right now Cider works in a single master multiple slaves manner, so to start
using Cider, a master node must be run somewhere using `cider master`. Once
a master node is running, `cider slave` can be used to spawn build slaves. All
that is necessary is to go to the machines that are to be used as build slaves
and run `cider slave` there. See the subcommands help for more details.

## Cider Internals ##

Cider uses [Meeko](http://meeko.io) RPC framework. Cider itself is
then rather simple. Cider master is a Cider RPC broker, the slaves are Cider
RPC clients which register certain RPC methods. `cider build` then generates
proper RPC method name and arguments, connects to the Cider broker and calls
the relevant method. The output steaming is actually implemented in Cider,
Cider gets this functionality for free.

### Build Request Routing ###

You might be wondering how the RPC requests are routed to the build slaves. It
is rather simple. The RPC method name is generated from the specified slave
label and the script runner to be used. It looks like `cider.SLAVE.RUNNER`.
All that is required from the particular Cider instance is that there is a
build slave connected, having SLAVE label assigned and being able to run RUNNER.

## Documentation ##

The help output of the `cider` command itself is rather verbose, so the best
thing to do is to run `cider -h` and see what is printed.

## Example ##

See the [demo repository](https://github.com/cider/cider-example).

## Benchmarks ##

See the `benchmark` subdirectory for more details.

## Discussion ##

You can join the [mailing list](https://groups.google.com/forum/#!forum/ciderci).

## License ##

MIT, see the `LICENSE` file.
