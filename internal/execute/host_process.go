package execute

import (
	"os"
	"time"
)

const hostWaitDelay = 250 * time.Millisecond

type hostProcessTree interface {
	Attach(*os.Process) error
	Cancel(*os.Process) error
	Close() error
}
