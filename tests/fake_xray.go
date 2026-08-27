package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
)

type cfg struct {
	Log struct {
		Access string `json:"access"`
	} `json:"log"`
	Inbounds []struct {
		Listen string `json:"listen"`
		Port   int    `json:"port"`
		Tag    string `json:"tag"`
	} `json:"inbounds"`
}

func main() {
	args := os.Args[1:]
	for _, a := range args {
		if a == "version" {
			fmt.Println("Xray 99.99.99 fake")
			return
		}
	}
	configPath := ""
	test := false
	for i, a := range args {
		if a == "-test" {
			test = true
		}
		if a == "-config" && i+1 < len(args) {
			configPath = args[i+1]
		}
	}
	if configPath == "" {
		os.Exit(2)
	}
	data, e := os.ReadFile(configPath)
	if e != nil {
		panic(e)
	}
	var c cfg
	if e = json.Unmarshal(data, &c); e != nil {
		panic(e)
	}
	if test {
		return
	}
	if len(c.Inbounds) == 0 {
		os.Exit(2)
	}
	ib := c.Inbounds[0]
	addr := net.JoinHostPort(ib.Listen, strconv.Itoa(ib.Port))
	ln, e := net.Listen("tcp", addr)
	if e != nil {
		panic(e)
	}
	defer ln.Close()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	go func() { <-sig; ln.Close() }()
	for {
		conn, e := ln.Accept()
		if e != nil {
			return
		}
		go handle(conn, ib.Tag, c.Log.Access)
	}
}
func handle(c net.Conn, inTag, logPath string) {
	defer c.Close()
	head := make([]byte, 2)
	if _, e := io.ReadFull(c, head); e != nil {
		return
	}
	methods := make([]byte, int(head[1]))
	if _, e := io.ReadFull(c, methods); e != nil {
		return
	}
	_, _ = c.Write([]byte{5, 0})
	req := make([]byte, 4)
	if _, e := io.ReadFull(c, req); e != nil {
		return
	}
	host := ""
	switch req[3] {
	case 1:
		x := make([]byte, 4)
		if _, e := io.ReadFull(c, x); e != nil {
			return
		}
		host = net.IP(x).String()
	case 4:
		x := make([]byte, 16)
		if _, e := io.ReadFull(c, x); e != nil {
			return
		}
		host = net.IP(x).String()
	case 3:
		n := make([]byte, 1)
		if _, e := io.ReadFull(c, n); e != nil {
			return
		}
		x := make([]byte, int(n[0]))
		if _, e := io.ReadFull(c, x); e != nil {
			return
		}
		host = string(x)
	default:
		return
	}
	pb := make([]byte, 2)
	if _, e := io.ReadFull(c, pb); e != nil {
		return
	}
	port := int(binary.BigEndian.Uint16(pb))
	up, e := net.Dial("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if e != nil {
		_, _ = c.Write([]byte{5, 5, 0, 1, 0, 0, 0, 0, 0, 0})
		return
	}
	defer up.Close()
	_, _ = c.Write([]byte{5, 0, 0, 1, 0, 0, 0, 0, 0, 0})
	if logPath != "" {
		if f, e := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644); e == nil {
			fmt.Fprintf(f, "tcp:%s accepted [%s -> direct]\n", net.JoinHostPort(host, strconv.Itoa(port)), inTag)
			f.Close()
		}
	}
	done := make(chan struct{}, 1)
	go func() { _, _ = io.Copy(up, c); done <- struct{}{} }()
	_, _ = io.Copy(c, up)
	<-done
}
