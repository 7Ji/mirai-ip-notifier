package main

import (
	"fmt"
)

func reader(done chan struct{}) {
	defer close(done)
	fmt.Println("Go routine hey")
	done <- struct{}{}
	fmt.Println("Go routine second")
}

func main() {
	fmt.Println("hello world")
	done := make(chan struct{})
	go reader(done)
	for {
		fmt.Println("New loop")
		select {
		case <-done:
			fmt.Println("We are done, exiting")
			return
		}
	}
}
