// Copyright (c) 2014 The cider AUTHORS
//
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

package master

import (
	// Stdlib
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	// Meeko
	"github.com/meeko/meekod/broker"
	broklog "github.com/meeko/meekod/broker/log"
	"github.com/meeko/meekod/daemon"

	// Others
	"github.com/cihub/seelog"
	"github.com/tchap/gocli"
)

var Command = &gocli.Command{
	UsageLine: "master [-debug] [-config=PATH]",
	Short:     "start a Cider build master node",
	Long: `
  Start a build master node using the meekod-compatible configuration file
  that can be found at PATH.

ENVIRONMENT:
  CIDER_MASTER_CONFIG - can be used instead of -config
	`,
	Action: run,
}

var (
	flagDebug  bool
	flagConfig string
)

func init() {
	Command.Flags.BoolVar(&flagDebug, "debug", flagDebug, "enable debug output")
	Command.Flags.StringVar(&flagConfig, "config", flagConfig, "meekod-compatible config file")
}

func run(cmd *gocli.Command, args []string) {
	if err := runWrapper(cmd, args); err != nil {
		log.Fatalln("\nError:", err)
	}
}

func runWrapper(cmd *gocli.Command, args []string) error {
	// Set up logging.
	log.SetFlags(0)
	if flagDebug {
		broklog.UseLogger(seelog.Default)
	} else {
		seelog.ReplaceLogger(seelog.Disabled)
	}

	// Make sure there were no arguments specified.
	if len(args) != 0 {
		cmd.Usage()
		os.Exit(2)
	}

	// Process input arguments and parameters.
	if flagConfig == "" {
		flagConfig = os.Getenv("CIDER_MASTER_CONFIG")
		if flagConfig == "" {
			log.Fatalln("Error: configuration file not specified")
		}
	}

	// Create a daemon instance from the specified config file.
	dmn, err := daemon.NewFromConfigAsFile(flagConfig, &daemon.Options{
		DisableSupervisor:     true,
		DisableLocalEndpoints: true,
	})
	if err != nil {
		return err
	}
	defer func() {
		dmn.Terminate()
		log.Println("The background thread terminated")
	}()

	// Start catching signals.
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)

	// Register a monitoring channel.
	monitorCh := make(chan *broker.EndpointCrashReport)
	dmn.Monitor(monitorCh)

	// Start the daemon.
	log.Println("Starting the background thread")
	dmnErrorCh := make(chan error, 1)
	go func() {
		dmnErrorCh <- dmn.Serve()
	}()

	// Loop until interrupted.
	for {
		select {
		case report, ok := <-monitorCh:
			if !ok {
				continue
			}
			if report.Dropped {
				return fmt.Errorf("Endpoint %v dropped", report.FactoryId)
			}
			log.Printf("Endpoint %v crashed: %v\n", report.FactoryId, report.Error)

		case err := <-dmnErrorCh:
			return err

		case <-signalCh:
			log.Println("Signal received, terminating...")
			return nil
		}
	}

	return nil
}
