package main

/*
#include <stdlib.h>
#include <string.h>

typedef uintptr_t DissectHandle;

// C-compatible structure for player stats within a round
typedef struct {
    char username[64];
    char operator_name[64];
    int team_index;
    int kills;
    int deaths;
    int assists;
    int headshots;
} CPlayerStat;

// C-compatible structure for team outcomes
typedef struct {
    char name[64];
    int score;
    int won;
    char win_condition[64];
} CTeamStat;
*/
import "C"
import (
	"fmt"
	"os"
	"runtime/cgo"
	"unsafe"

	"github.com/redraskal/r6-dissect/dissect"
	"github.com/rs/zerolog"
)

func init() {
	zerolog.SetGlobalLevel(zerolog.Disabled)
}

// Safely copies a Go string into a fixed-size C char array.
func copyString(dest *[64]C.char, src string) {
	length := len(src)
	if length > 63 {
		length = 63
	}
	for i := 0; i < length; i++ {
		dest[i] = C.char(src[i])
	}
	dest[length] = 0 // Null terminator
}

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

	// r is already *dissect.Reader. Pass without the address operator.
	return C.DissectHandle(cgo.NewHandle(r))
}

//export Dissect_Free
func Dissect_Free(h C.DissectHandle) {
	if h == 0 {
		return
	}
	cgo.Handle(h).Delete()
}

//export Dissect_GetSite
func Dissect_GetSite(h C.DissectHandle) *C.char {
	if h == 0 {
		return nil
	}
	r := cgo.Handle(h).Value().(*dissect.Reader)
	return C.CString(r.Header.Site)
}

//export Dissect_FreeString
func Dissect_FreeString(str *C.char) {
	if str != nil {
		C.free(unsafe.Pointer(str))
	}
}

//export Dissect_GetTeams
func Dissect_GetTeams(h C.DissectHandle, outCount *C.int) *C.CTeamStat {
	if h == 0 {
		*outCount = 0
		return nil
	}
	r := cgo.Handle(h).Value().(*dissect.Reader)

	count := len(r.Header.Teams)
	*outCount = C.int(count)
	if count == 0 {
		return nil
	}

	// Allocate array on C heap to return ownership to C
	cArray := (*C.CTeamStat)(C.malloc(C.size_t(count) * C.size_t(unsafe.Sizeof(C.CTeamStat{}))))
	cSlice := (*[1 << 28]C.CTeamStat)(unsafe.Pointer(cArray))[:count:count]

	for i, t := range r.Header.Teams {
		copyString(&cSlice[i].name, t.Name)
		cSlice[i].score = C.int(t.Score)

		if t.Won {
			cSlice[i].won = 1
		} else {
			cSlice[i].won = 0
		}

		// Coerce WinCondition to string to avoid underlying type mismatch
		copyString(&cSlice[i].win_condition, fmt.Sprintf("%v", t.WinCondition))
	}

	return cArray
}

//export Dissect_GetPlayerStats
func Dissect_GetPlayerStats(h C.DissectHandle, outCount *C.int) *C.CPlayerStat {
	if h == 0 {
		*outCount = 0
		return nil
	}
	r := cgo.Handle(h).Value().(*dissect.Reader)

	count := len(r.Header.Players)
	*outCount = C.int(count)
	if count == 0 {
		return nil
	}

	cArray := (*C.CPlayerStat)(C.malloc(C.size_t(count) * C.size_t(unsafe.Sizeof(C.CPlayerStat{}))))
	cSlice := (*[1 << 28]C.CPlayerStat)(unsafe.Pointer(cArray))[:count:count]

	// Map username to index for O(1) aggregation during feedback loop
	playerIndexMap := make(map[string]int)

	for i, p := range r.Header.Players {
		copyString(&cSlice[i].username, p.Username)
		copyString(&cSlice[i].operator_name, p.Operator.String())
		cSlice[i].team_index = C.int(p.TeamIndex)
		cSlice[i].kills = 0
		cSlice[i].deaths = 0
		cSlice[i].assists = 0
		cSlice[i].headshots = 0
		playerIndexMap[p.Username] = i
	}

	// Parse MatchFeedback array to aggregate Kills, Deaths, and Headshots
	for _, fb := range r.MatchFeedback {
		if fb.Type == dissect.Kill {
			// Credit Killer
			if idx, ok := playerIndexMap[fb.Username]; ok {
				cSlice[idx].kills++
				if *fb.Headshot {
					cSlice[idx].headshots++
				}
			}
			// Credit Victim
			if idx, ok := playerIndexMap[fb.Target]; ok {
				cSlice[idx].deaths++
			}
		}
	}

	return cArray
}

func main() {}
