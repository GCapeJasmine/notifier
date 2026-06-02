package utils

import (
	"fmt"
	"os"

	"github.com/urfave/cli/v2"
	"go.uber.org/zap"

	"github.com/gleo/subscribers/common/log"
	"github.com/gleo/subscribers/common/utils"
)

const filePathFlag = "config"

func RunMain[T any](application string, runner func(cfg T) error) {
	flags := []cli.Flag{
		&cli.StringFlag{
			Name:  filePathFlag,
			Usage: "config file location, including filename",
		},
	}
	app := &cli.App{
		Name:  application,
		Flags: flags,
		Action: func(cli *cli.Context) (err error) {
			defer func() {
				if errR := recover(); errR != nil {
					log.Logger.Errorw("application occurs panic", "error", errR)
					err = fmt.Errorf("application occurs panic: %v", errR)
				}
			}()

			cfg := readConfig[T](cli.String(filePathFlag))
			log.Logger.Infow("initializing application with", "config", cfg)

			err = runner(cfg)
			if err != nil {
				log.Logger.Errorw("cannot start application", zap.Error(err))
			}
			return
		},
		Before: func(cli *cli.Context) error {
			return nil
		},
		After: func(cli *cli.Context) error {
			log.Logger.Info("terminate application, flushing logs")
			return nil
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Logger.Fatal(err)
	}
}

func readConfig[T any](filePath string) T {
	var configuration T
	utils.LoadConfig(filePath, &configuration)
	return configuration
}
