package main

import (
	"fmt"
	"time"
)

const MountPath = "/media"
const SymlinkPath = "/mnt/Ishamedia"


func main() {
	fmt.Println("Test")

	ticker := time.NewTicker(1000 * time.Millisecond)
	quit := make(chan struct{})
	InitMountDir()

	go func() {
		for {
			select {
			case <-ticker.C:
				ObserveBlockDev()
			case <-quit:
				ticker.Stop()
				return
			}
		}
	}()

	select {}
}
