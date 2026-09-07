package main

import "flag"

type Config struct {
	Hostname     string
	ListenAddr   string
	DKIMKeyPath  string
	DKIMSelector string
}

func LoadConfig() *Config {
	cfg := &Config{}
	flag.StringVar(&cfg.Hostname, "hostname", "", "The hostname this SMTP server acts as")
	flag.StringVar(&cfg.ListenAddr, "listen", "127.0.0.1:2525", "The listen address the server runs on")

	flag.Parse()

	return cfg
}
