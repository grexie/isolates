package main

import (
	_ "embed"
	"io"
	"log"
	"os"
	"os/exec"
)

//go:embed install-v8.sh
var script []byte

func main() {
	args := append([]string{"-es", "-"}, os.Args[1:]...)
	cmd := exec.Command("/bin/bash", args...)
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	stdin, _ := cmd.StdinPipe()
	cmd.Start()
	go io.Copy(os.Stdout, stdout)
	go io.Copy(os.Stderr, stderr)
	stdin.Write(script)
	stdin.Close()
	if err := cmd.Wait(); err != nil {
		log.Println(err)
	}
}
