package kafka

import (
	"fmt"
	"strings"
	"time"

	"github.com/segmentio/kafka-go"

	"github.com/gleo/subscribers/common/utils"
	"github.com/segmentio/kafka-go/sasl/plain"
	"github.com/segmentio/kafka-go/sasl/scram"
)

func checkConn(dialer *kafka.Dialer, url string) error {
	conn, err := dialer.Dial("tcp", url)
	if err != nil {
		return err
	}
	defer conn.Close()
	return nil
}

func NewDialer(kafkaCnf IAuthConfig) *kafka.Dialer {
	dialer := kafka.DefaultDialer
	if kafkaCnf.GetUsername() != "" {
		var err error
		switch strings.ToUpper(kafkaCnf.GetAuthentication()) {
		case scram.SHA512.Name():
			dialer.SASLMechanism, err = scram.Mechanism(scram.SHA512, kafkaCnf.GetUsername(), kafkaCnf.GetPassword())
		case scram.SHA256.Name():
			dialer.SASLMechanism, err = scram.Mechanism(scram.SHA256, kafkaCnf.GetUsername(), kafkaCnf.GetPassword())
		default:
			dialer.SASLMechanism = plain.Mechanism{Username: kafkaCnf.GetUsername(), Password: kafkaCnf.GetPassword()}
		}
		if err != nil {
			panic(fmt.Sprintf("error while get kafka dialer: %v", err.Error()))
		}
	}
	if kafkaCnf.GetTimeout() > 0 {
		dialer.Timeout = time.Duration(kafkaCnf.GetTimeout()) * time.Millisecond
	}

	for _, addr := range utils.SplitAndTrim(kafkaCnf.GetUrl()) {
		if err := checkConn(dialer, addr); err != nil {
			panic(err)
		}
	}

	return dialer
}
