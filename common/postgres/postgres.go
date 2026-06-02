package postgres

import (
	"fmt"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type Config struct {
	DriverName string `yaml:"driver_name" mapstructure:"driver_name"`
	Host       string `yaml:"host" mapstructure:"host"`
	Port       int    `yaml:"port" mapstructure:"port"`
	Username   string `yaml:"username" mapstructure:"username" json:"-"`
	Password   string `yaml:"password" mapstructure:"password" json:"-"`
	Database   string `yaml:"database" mapstructure:"database"`
	SslMode    string `yaml:"ssl_mode" mapstructure:"ssl_mode"`
}

func (config Config) DSN() string {
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		config.Host, config.Port, config.Username, config.Password, config.Database, config.SslMode)
}

func NewGormDB(config Config) (*gorm.DB, error) {
	return gorm.Open(postgres.New(
		postgres.Config{
			DriverName: config.DriverName,
			DSN:        config.DSN(),
		},
	))
}

func NewGormDBFailFast(config Config) *gorm.DB {
	db, err := NewGormDB(config)
	if err != nil {
		panic(err)
	}
	return db
}
