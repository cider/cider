// Copyright (c) 2014 The cider AUTHORS
//
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

package master

import (
	// Stdlib
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

var configPath string

var Command = &gocli.Command{
	UsageLine: "master [-config=PATH]",
	Short:     "start a Cider build master node",
	Long: `
  Start a build master node using the meekod-compatible configuration file
  that can be found at PATH.

ENVIRONMENT:
  CIDER_MASTER_CONFIG - can be used instead of -config
	`,
	Action: run,
}

func init() {
	Command.Flags.StringVar(&configPath, "config", configPath, "meekod-compatible config file")
}

func run(cmd *gocli.Command, args []string) {
	if err := runWrapper(cmd, args); err != nil {
		log.Fatalln("Error: ", err)
	}
}

func runWrapper(cmd *gocli.Command, args []string) error {
	// Set up logging.
	log.SetFlags(0)
	broklog.UseLogger(seelog.Default)

	// Make sure there were no arguments specified.
	if len(args) != 0 {
		cmd.Usage()
		os.Exit(2)
	}

	// Process input arguments and parameters.
	if configPath == "" {
		configPath = os.Getenv("CIDER_MASTER_CONFIG")
		if configPath == "" {
			log.Fatalln("Error: configuration file not specified")
		}
	}

	// Create a daemon instance from the specified config file.
	dmn, err := daemon.NewFromConfigAsFile(configPath, &daemon.Options{
		DisableSupervisor:     true,
		DisableLocalEndpoints: true,
	})
	if err != nil {
		return err
	}
	defer dmn.Terminate()

	// Start catching signals.
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)

	// Register a monitoring channel.
	monitorCh := make(chan *broker.EndpointCrashReport)
	dmn.Monitor(monitorCh)

	// Loop until interrupted.
	for {
		select {
		case report, ok := <-monitorCh:
			if report != nil && report.Dropped {
				log.Printf("Endpoint %v dropped", report.FactoryId)
				return report.Error
			}
			if !ok {
				return nil
			}
			log.Printf("Endpoint %v crashed with error=%v\n", report.FactoryId, report.Error)

		case sig := <-signalCh:
			log.Printf("Signal received (%v), terminating...\n", sig)
			return nil
		}
	}

	return nil
}
