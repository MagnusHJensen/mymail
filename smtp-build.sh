#!/bin/sh

GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o mymail-smtp ./packages/smtp