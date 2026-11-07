//go:build !desktop

package main

func runPanelWindow(_ string, _ string, stopped <-chan struct{}) error {
	<-stopped
	return nil
}
