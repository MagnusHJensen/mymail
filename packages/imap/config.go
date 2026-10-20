package main

import (
	"flag"
)

type Config struct {
	Debug bool

	MailDomain string

	TLSCertFile string
	TLSKeyFile  string

	ListenAddr string
}

func LoadConfig() *Config {
	cfg := &Config{}
	flag.BoolVar(&cfg.Debug, "debug", false, "Whether to start in debug mode")

	flag.StringVar(&cfg.TLSCertFile, "tls-cert-file", "", "The TLS cert file")
	flag.StringVar(&cfg.TLSKeyFile, "tls-key-file", "", "The TLS key file")

	flag.StringVar(&cfg.ListenAddr, "listen", "127.0.0.1:2626", "The listen address the server runs on")

	flag.Parse()

	return cfg
}
