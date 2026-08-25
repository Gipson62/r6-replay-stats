package main

/*
#include <stdlib.h>
#include <stdint.h>

// One malloc'd instance per Dissect_Get* call; each has a paired
// Dissect_Free* that releases every string field plus the struct itself.
// "type" is a Go keyword, hence "event_type" below -- keeps the Go and C
// sides using the identical field name instead of cgo's keyword-collision
// renaming rules.

typedef struct {
    char* match_id;
    char* map_name;
    char* game_mode;
    char* game_version;
    char* site;
    int32_t round_number;
    int32_t rounds_per_match;
} DissectHeader;

typedef struct {
    char* name;
    int32_t starting_score;
    int32_t score;
    int32_t won;            // 0/1
    char* win_condition;    // "KilledOpponents"/"SecuredArea"/"DisabledDefuser"/
                             // "DefusedBomb"/"ExtractedHostage"/"Time"/"" if undecided
    char* role;              // "Attack"/"Defense"
} DissectTeam;

typedef struct {
    char* username;
    int32_t team_index;
    char* operator_name;
} DissectPlayer;

typedef struct {
    char* event_type;       // "Kill", "Death", "DefuserPlantComplete", ...
    char* username;
    char* target;
    int32_t headshot;       // -1 = not applicable (source Headshot was nil), else 0/1
    double time_in_seconds;
    char* operator_name;    // actor's operator at the time of this event
} DissectEvent;
*/
import "C"

import (
	"os"
	"runtime/cgo"
	"unsafe"

	"github.com/redraskal/r6-dissect/dissect"
	"github.com/rs/zerolog"
)

func init() {
	zerolog.SetGlobalLevel(zerolog.Disabled)
}

func freeCStr(s *C.char) {
	if s != nil {
		C.free(unsafe.Pointer(s))
	}
}

// --- handle lifecycle ---

// Dissect_Open ingests a round file path, runs dissect.NewReader + Reader.Read,
// and returns an opaque handle to the resulting *dissect.Reader.
// Returns 0 on any failure (open error, invalid file, read error).
//
//export Dissect_Open
func Dissect_Open(path *C.char) C.uintptr_t {
	if path == nil {
		return 0
	}
	goPath := C.GoString(path)

	f, err := os.Open(goPath)
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

	h := cgo.NewHandle(r)
	return C.uintptr_t(h)
}

// Dissect_Free releases the cgo.Handle backing a *dissect.Reader.
// Must be called exactly once per successful Dissect_Open. Safe on invalid handles.
//
//export Dissect_Free
func Dissect_Free(handle C.uintptr_t) {
	defer func() { recover() }()
	cgo.Handle(handle).Delete()
}

// readerFromHandle resolves a handle to its *dissect.Reader.
// Recovers from panics raised by an invalid, stale, or zero handle.
func readerFromHandle(handle C.uintptr_t) (r *dissect.Reader, ok bool) {
	defer func() {
		if recover() != nil {
			r, ok = nil, false
		}
	}()
	v := cgo.Handle(handle).Value()
	r, ok = v.(*dissect.Reader)
	return
}

// --- counts: still needed to know how many indices are valid ---

//export Dissect_PlayerCount
func Dissect_PlayerCount(handle C.uintptr_t) C.int32_t {
	r, ok := readerFromHandle(handle)
	if !ok {
		return -1
	}
	return C.int32_t(len(r.Header.Players))
}

//export Dissect_EventCount
func Dissect_EventCount(handle C.uintptr_t) C.int32_t {
	r, ok := readerFromHandle(handle)
	if !ok {
		return -1
	}
	return C.int32_t(len(r.MatchFeedback))
}

// --- struct getters ---
// Each returns a malloc'd, caller-owned struct (nil on invalid handle /
// out-of-bounds index), released via the paired Dissect_Free* function.

// Dissect_GetHeader returns this round file's match/header metadata,
// including the round number and site recorded for this specific round.
//
//export Dissect_GetHeader
func Dissect_GetHeader(handle C.uintptr_t) *C.DissectHeader {
	r, ok := readerFromHandle(handle)
	if !ok {
		return nil
	}
	out := (*C.DissectHeader)(C.malloc(C.size_t(C.sizeof_DissectHeader)))
	out.match_id = C.CString(r.Header.MatchID)
	out.map_name = C.CString(r.Header.Map.String())
	out.game_mode = C.CString(r.Header.GameMode.String())
	out.game_version = C.CString(r.Header.GameVersion)
	out.site = C.CString(r.Header.Site)
	out.round_number = C.int32_t(r.Header.RoundNumber)
	out.rounds_per_match = C.int32_t(r.Header.RoundsPerMatch)
	return out
}

//export Dissect_FreeHeader
func Dissect_FreeHeader(h *C.DissectHeader) {
	if h == nil {
		return
	}
	freeCStr(h.match_id)
	freeCStr(h.map_name)
	freeCStr(h.game_mode)
	freeCStr(h.game_version)
	freeCStr(h.site)
	C.free(unsafe.Pointer(h))
}

// Dissect_GetTeam returns team `index`'s (0 or 1) data for this round,
// including its name, score, whether it won, and (if it won) how.
//
//export Dissect_GetTeam
func Dissect_GetTeam(handle C.uintptr_t, index C.int32_t) *C.DissectTeam {
	r, ok := readerFromHandle(handle)
	if !ok {
		return nil
	}
	i := int(index)
	if i < 0 || i >= len(r.Header.Teams) {
		return nil
	}
	t := r.Header.Teams[i]
	out := (*C.DissectTeam)(C.malloc(C.size_t(C.sizeof_DissectTeam)))
	out.name = C.CString(t.Name)
	out.starting_score = C.int32_t(t.StartingScore)
	out.score = C.int32_t(t.Score)
	if t.Won {
		out.won = 1
	} else {
		out.won = 0
	}
	out.win_condition = C.CString(string(t.WinCondition))
	out.role = C.CString(string(t.Role))
	return out
}

//export Dissect_FreeTeam
func Dissect_FreeTeam(t *C.DissectTeam) {
	if t == nil {
		return
	}
	freeCStr(t.name)
	freeCStr(t.win_condition)
	freeCStr(t.role)
	C.free(unsafe.Pointer(t))
}

// Dissect_GetPlayer returns participant `index`'s data for this round:
// username, team index, and the operator they played this round.
//
//export Dissect_GetPlayer
func Dissect_GetPlayer(handle C.uintptr_t, index C.int32_t) *C.DissectPlayer {
	r, ok := readerFromHandle(handle)
	if !ok {
		return nil
	}
	i := int(index)
	if i < 0 || i >= len(r.Header.Players) {
		return nil
	}
	p := r.Header.Players[i]
	out := (*C.DissectPlayer)(C.malloc(C.size_t(C.sizeof_DissectPlayer)))
	out.username = C.CString(p.Username)
	out.team_index = C.int32_t(p.TeamIndex)
	out.operator_name = C.CString(p.Operator.String())
	return out
}

//export Dissect_FreePlayer
func Dissect_FreePlayer(p *C.DissectPlayer) {
	if p == nil {
		return
	}
	freeCStr(p.username)
	freeCStr(p.operator_name)
	C.free(unsafe.Pointer(p))
}

// Dissect_GetEvent returns match-feedback entry `index`: its type, the
// primary/secondary actors, headshot (-1 if not applicable to this event
// type), timestamp, and the actor's operator at the time.
//
//export Dissect_GetEvent
func Dissect_GetEvent(handle C.uintptr_t, index C.int32_t) *C.DissectEvent {
	r, ok := readerFromHandle(handle)
	if !ok {
		return nil
	}
	i := int(index)
	if i < 0 || i >= len(r.MatchFeedback) {
		return nil
	}
	e := r.MatchFeedback[i]
	out := (*C.DissectEvent)(C.malloc(C.size_t(C.sizeof_DissectEvent)))
	out.event_type = C.CString(e.Type.String())
	out.username = C.CString(e.Username)
	out.target = C.CString(e.Target)
	if e.Headshot == nil {
		out.headshot = -1
	} else if *e.Headshot {
		out.headshot = 1
	} else {
		out.headshot = 0
	}
	out.time_in_seconds = C.double(e.TimeInSeconds)
	out.operator_name = C.CString(e.Operator.String())
	return out
}

//export Dissect_FreeEvent
func Dissect_FreeEvent(e *C.DissectEvent) {
	if e == nil {
		return
	}
	freeCStr(e.event_type)
	freeCStr(e.username)
	freeCStr(e.target)
	freeCStr(e.operator_name)
	C.free(unsafe.Pointer(e))
}

func main() {}
