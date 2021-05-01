/*
	The package system contains only one function, which should be instantiated as a Go routine: it listens to OS
	signals like SIGINT and SIGTEM, and when it receives a signal, it closes the channel given as the sole argument to
	notify the caller routine.

	Note: The use case is when a user wants to terminate Rabia (in case Rabia errs) before it normally exits.
*/
package system

import (
	"os"
	"os/signal"
	"syscall"
)

/*
	When SigListen is spawned as a Go routine by a caller routine, SigListen listens to SIGINT and SIGTERM. So when a
	user presses ctrl+c or the OS sends a kill signal, SigListen closes the done channel to notify the caller routine
	to exit.

	done: a channel created by the caller routine, the typical usage is like:
		at the caller routine:
			// initialize the done channel
			done := make(chan struct{})
			go SigListen(done)
			select {
			case <- done: // A receive from a closed channel returns the zero value immediately
				return
			case ...
			}
*/
func SigListen(done chan struct{}) {
	sigIn := make(chan os.Signal, 1)
	signal.Notify(sigIn, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-done:
		signal.Stop(sigIn)
	case <-sigIn:
		signal.Stop(sigIn)
		close(done)
	}
}
