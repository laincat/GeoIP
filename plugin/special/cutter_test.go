package special

import (
	"fmt"
	"testing"
	"github.com/laincat/GeoIP/lib"
)

func TestCutterSnapshot(t *testing.T) {
	for round := 0; round < 10; round++ {
		container := lib.NewContainer()
		for n := 0; n < 1000; n++ {
			entry := lib.NewEntry(fmt.Sprintf("TEST%d", n))
			if err := entry.AddPrefix("192.0.2.0/24"); err != nil { t.Fatal(err) }
			if err := container.Add(entry); err != nil { t.Fatal(err) }
		}
		cutter := &Cutter{Action: lib.ActionRemove}
		if _, err := cutter.Input(container); err != nil { t.Fatal(err) }
		if container.Len() != 0 { t.Fatal("entries remain") }
	}
}
