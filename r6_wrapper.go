package main

/*
#include <stdlib.h>
typedef uintptr_t DissectHandle;
*/
import "C"
import (
	"os"
	"runtime/cgo"

	"github.com/Gipson62/r6-dissect/dissect"
)

//export Dissect_Open
func Dissect_Open(filePath *C.char) C.DissectHandle {
	path := C.GoString(filePath)

	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()

	r, err := dissect.NewReader(f)
	if err != nil {
		return 0
	}

	if err := r.Read(); !dissect.Ok(err) {
		return 0
	}

	return C.DissectHandle(cgo.NewHandle(&r))
}

//export Dissect_Free
func Dissect_Free(h C.DissectHandle) {
	if h == 0 {
		return
	}
	cgo.Handle(h).Delete()
}

func main() {}