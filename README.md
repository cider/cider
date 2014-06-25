# Cider

Cider is a framework for implementing continuous integration servers.

## Overview

Cider is a set of [Meeko](http://meeko.io) agents that can be used to implement
a simple continuous integration (CI) server.

Cider itself contains:

* the build slave agent, and
* the build trigger agent.

Meeko itself acts as the build master component.

Right now there is no component that handles the build output, this must be implemented in
some other agent. This is actually not that critical as it may seem, check the Tips and Tricks
section.

### The Build Slave Agent

The first and the most important component that is included is the build slave agent.
It exports certain methods over the Meeko RPC service. These methods can be then called
by other agents to trigger builds on the relevant build slaves.

Every build slave is associated with a set of labels and a set of runners. The labels are
just some user-defined strings that represent the environment of the build slave. It can be
e.g. `linux-ubuntu-14.04` or `macosx-10.9`. The set of runners is generated when the slave
is started and it describes what script executors are available on the target system.
The available runners are:

* `bash` - bash
* `node` - node
* `powershell` - PowerShell.exe
* `cmd` - cmd.exe

The slave automatically activates the runners that can be found in `PATH`.

Once the labels and runners are known, the build slave connects to the Meeko RPC service
and it exports methods schematically looking like `cider.LABEL.RUNNER`. The slave just
does a Cartesian product, so the number of methods exported is `|labels| * |runners|`.

Certain information must be supplied as the method arguments:

| Name            | Type       | Description                                                    |
| --------------- |:----------:| -------------------------------------------------------------- |
| `repository`    | `string`   | Meeko-compatible repository URL                                |
| `script`        | `string`   | the relative path of the script to be executed                 |
| `env`           | `[]string` | the list of environment variables to be defined for the script |

The build slave then clones/pulls the specified repository and uses the relevant runner to run
the specified script. The variables defined in `env` are exported for the build script.
The build output is being streamed back to the requested using the RPC service. Once the build
is finished, the following value is returned

| Name            | Type            | Description                       |
| --------------- |:---------------:| --------------------------------- |
| `pullDuration`  | `time.Duration` | time spent pulling the repository |
| `buildDuration` | `time.Duration` | time spent running the script     |
| `error`         | `string`        | error message, if any             |

The return code is `0` on success, `1` on failure.

### The Build Trigger Agent

The second agent, available as `cider build` subcommand, can be used to trigger builds remotely.
The usage is explained in the [example repository](https://github.com/cider/cider-example).

## Installation ##

You will need [Go](http://golang.org) 1.1 or higher.

## Usage ##

The help output of the `cider` command is rather verbose, so the best
thing to do is to run `cider -h`.

## Example ##

See the [example repository](https://github.com/cider/cider-example).

## Benchmarks ##

See the `benchmark` subdirectory for more details.

## Discussion ##

You can join the [mailing list](https://groups.google.com/forum/#!forum/ciderci).

## Status ##

This is very very much still in development, use at your own risk.

## Tips and Tricks ##

Cider can be used to simply add build slaves to other CI servers. All that is necessary
is to run `cider build` from within the other CI server. That will stream the build output to
the console and the other CI server will take care of saving of the build output. This is very
handy in case you fancy certain CI server solution, but you need a build slave environment that
is not supported. This happens quite often with the hosted CI solutions. Usually only Linux build
slaves are supported.

## Contributing ##

See `CONTRIBUTING.md`.

## License ##

MIT, see the `LICENSE` file.

## About the Original Authors ##

[tchap](https://github.com/tchap) started with this project because he was too fed up with other
continuous integration servers.

Cider is going to be used at [Salsita](https://github.com/salsita).
