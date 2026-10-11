package main

import (
	"flag"
	"log"
)

type Config struct {
	Debug bool

	Hostname   string
	MailDomain string

	NoSend bool

	TLSCertFile string
	TLSKeyFile  string

	ListenAddr     string
	SendListenAddr string

	DKIMKeyPath  string
	DKIMSelector string
}

func LoadConfig() *Config {
	cfg := &Config{}
	flag.BoolVar(&cfg.Debug, "debug", false, "Whether to start in debug mode")

	flag.BoolVar(&cfg.NoSend, "no-send", false, "If true does not send any emails out only accepts incoming")

	flag.StringVar(&cfg.MailDomain, "mail-domain", "", "The mail domain for recipient, sender and maildir checks.")
	flag.StringVar(&cfg.Hostname, "hostname", "", "The hostname this SMTP server acts as")

	flag.StringVar(&cfg.TLSCertFile, "tls-cert-file", "", "The TLS cert file")
	flag.StringVar(&cfg.TLSKeyFile, "tls-key-file", "", "The TLS key file")

	flag.StringVar(&cfg.ListenAddr, "listen", "127.0.0.1:2525", "The listen address the server runs on")
	flag.StringVar(&cfg.SendListenAddr, "send-listen", "127.0.0.1:2424", "The address to listen on for send connections")

	flag.Parse()

	// TODO: Add config validation
	if cfg.Hostname == "" || cfg.MailDomain == "" {
		log.Fatal("both -hostname and -mail-domain are required")
	}

	return cfg
}
