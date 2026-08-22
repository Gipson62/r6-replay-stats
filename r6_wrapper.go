package main

/*
#include <stdlib.h>
#include <stdint.h>
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

// Dissect_FreeString releases a *C.char allocated by C.CString in this wrapper.
// Must be called on every non-NULL string returned by the getters below.
//
//export Dissect_FreeString
func Dissect_FreeString(s *C.char) {
	if s == nil {
		return
	}
	C.free(unsafe.Pointer(s))
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

// playerRoundStats resolves the PlayerRoundStats entry matching the player
// at Header.Players[index], by username, for the current round.
func playerRoundStats(r *dissect.Reader, index int) (dissect.PlayerRoundStats, bool) {
	if index < 0 || index >= len(r.Header.Players) {
		return dissect.PlayerRoundStats{}, false
	}
	username := r.Header.Players[index].Username
	for _, s := range r.PlayerStats() {
		if s.Username == username {
			return s, true
		}
	}
	return dissect.PlayerRoundStats{}, false
}

// --- Header Metrics ---

//export Dissect_MatchID
func Dissect_MatchID(handle C.uintptr_t) *C.char {
	r, ok := readerFromHandle(handle)
	if !ok {
		return nil
	}
	return C.CString(r.Header.MatchID)
}

//export Dissect_Map
func Dissect_Map(handle C.uintptr_t) *C.char {
	r, ok := readerFromHandle(handle)
	if !ok {
		return nil
	}
	return C.CString(r.Header.Map.String())
}

//export Dissect_GameMode
func Dissect_GameMode(handle C.uintptr_t) *C.char {
	r, ok := readerFromHandle(handle)
	if !ok {
		return nil
	}
	return C.CString(r.Header.GameMode.String())
}

//export Dissect_GameVersion
func Dissect_GameVersion(handle C.uintptr_t) *C.char {
	r, ok := readerFromHandle(handle)
	if !ok {
		return nil
	}
	return C.CString(r.Header.GameVersion)
}

// --- Player Data ---

//export Dissect_PlayerCount
func Dissect_PlayerCount(handle C.uintptr_t) C.int {
	r, ok := readerFromHandle(handle)
	if !ok {
		return -1
	}
	return C.int(len(r.Header.Players))
}

//export Dissect_PlayerUsername
func Dissect_PlayerUsername(handle C.uintptr_t, index C.int) *C.char {
	r, ok := readerFromHandle(handle)
	if !ok {
		return nil
	}
	i := int(index)
	if i < 0 || i >= len(r.Header.Players) {
		return nil
	}
	return C.CString(r.Header.Players[i].Username)
}

//export Dissect_PlayerTeamIndex
func Dissect_PlayerTeamIndex(handle C.uintptr_t, index C.int) C.int {
	r, ok := readerFromHandle(handle)
	if !ok {
		return -1
	}
	i := int(index)
	if i < 0 || i >= len(r.Header.Players) {
		return -1
	}
	return C.int(r.Header.Players[i].TeamIndex)
}

//export Dissect_PlayerKills
func Dissect_PlayerKills(handle C.uintptr_t, index C.int) C.int {
	r, ok := readerFromHandle(handle)
	if !ok {
		return -1
	}
	s, ok := playerRoundStats(r, int(index))
	if !ok {
		return -1
	}
	return C.int(s.Kills)
}

// Dissect_PlayerDeaths returns 1 if the player died during this round, 0 if not,
// -1 on out-of-bounds. The underlying library tracks round death as a bool (Died),
// not a cumulative count; aggregate across rounds on the C side if needed.
//
//export Dissect_PlayerDeaths
func Dissect_PlayerDeaths(handle C.uintptr_t, index C.int) C.int {
	r, ok := readerFromHandle(handle)
	if !ok {
		return -1
	}
	s, ok := playerRoundStats(r, int(index))
	if !ok {
		return -1
	}
	if s.Died {
		return 1
	}
	return 0
}

//export Dissect_PlayerAssists
func Dissect_PlayerAssists(handle C.uintptr_t, index C.int) C.int {
	r, ok := readerFromHandle(handle)
	if !ok {
		return -1
	}
	s, ok := playerRoundStats(r, int(index))
	if !ok {
		return -1
	}
	return C.int(s.Assists)
}

//export Dissect_PlayerHeadshots
func Dissect_PlayerHeadshots(handle C.uintptr_t, index C.int) C.int {
	r, ok := readerFromHandle(handle)
	if !ok {
		return -1
	}
	s, ok := playerRoundStats(r, int(index))
	if !ok {
		return -1
	}
	return C.int(s.Headshots)
}

// --- Round Metrics ---
// dissect.Reader (from NewReader) represents a single round's data, not a
// multi-round match object — the underlying library has no Match.Rounds slice.
// RoundNumber/RoundsPerMatch expose the round's position within its match.

//export Dissect_RoundNumber
func Dissect_RoundNumber(handle C.uintptr_t) C.int {
	r, ok := readerFromHandle(handle)
	if !ok {
		return -1
	}
	return C.int(r.Header.RoundNumber)
}

//export Dissect_RoundsPerMatch
func Dissect_RoundsPerMatch(handle C.uintptr_t) C.int {
	r, ok := readerFromHandle(handle)
	if !ok {
		return -1
	}
	return C.int(r.Header.RoundsPerMatch)
}

// Dissect_RoundWinningTeamIndex returns the index (0 or 1) of the team marked
// Won for this round, or -1 if undetermined / handle invalid.
//
//export Dissect_RoundWinningTeamIndex
func Dissect_RoundWinningTeamIndex(handle C.uintptr_t) C.int {
	r, ok := readerFromHandle(handle)
	if !ok {
		return -1
	}
	for i, t := range r.Header.Teams {
		if t.Won {
			return C.int(i)
		}
	}
	return -1
}

//export Dissect_ObjectiveSite
func Dissect_ObjectiveSite(handle C.uintptr_t) *C.char {
	r, ok := readerFromHandle(handle)
	if !ok {
		return nil
	}
	return C.CString(r.Header.Site)
}

// --- Event Timeline (Match Feedback) ---

//export Dissect_EventCount
func Dissect_EventCount(handle C.uintptr_t) C.int {
	r, ok := readerFromHandle(handle)
	if !ok {
		return -1
	}
	return C.int(len(r.MatchFeedback))
}

// Dissect_EventType returns the string form of MatchUpdateType
// (Kill, Death, DefuserPlantStart, DefuserPlantComplete, DefuserDisableStart,
// DefuserDisableComplete, LocateObjective, OperatorSwap, Battleye, PlayerLeave, Other).
//
//export Dissect_EventType
func Dissect_EventType(handle C.uintptr_t, index C.int) *C.char {
	r, ok := readerFromHandle(handle)
	if !ok {
		return nil
	}
	i := int(index)
	if i < 0 || i >= len(r.MatchFeedback) {
		return nil
	}
	return C.CString(r.MatchFeedback[i].Type.String())
}

//export Dissect_EventTime
func Dissect_EventTime(handle C.uintptr_t, index C.int) *C.char {
	r, ok := readerFromHandle(handle)
	if !ok {
		return nil
	}
	i := int(index)
	if i < 0 || i >= len(r.MatchFeedback) {
		return nil
	}
	return C.CString(r.MatchFeedback[i].Time)
}

//export Dissect_EventTimeInSeconds
func Dissect_EventTimeInSeconds(handle C.uintptr_t, index C.int) C.double {
	r, ok := readerFromHandle(handle)
	if !ok {
		return -1
	}
	i := int(index)
	if i < 0 || i >= len(r.MatchFeedback) {
		return -1
	}
	return C.double(r.MatchFeedback[i].TimeInSeconds)
}

// Dissect_EventUsername returns the primary actor (killer, planter, disabler, locator).
//
//export Dissect_EventUsername
func Dissect_EventUsername(handle C.uintptr_t, index C.int) *C.char {
	r, ok := readerFromHandle(handle)
	if !ok {
		return nil
	}
	i := int(index)
	if i < 0 || i >= len(r.MatchFeedback) {
		return nil
	}
	return C.CString(r.MatchFeedback[i].Username)
}

// Dissect_EventTarget returns the secondary actor (victim, or empty string for
// events with no target).
//
//export Dissect_EventTarget
func Dissect_EventTarget(handle C.uintptr_t, index C.int) *C.char {
	r, ok := readerFromHandle(handle)
	if !ok {
		return nil
	}
	i := int(index)
	if i < 0 || i >= len(r.MatchFeedback) {
		return nil
	}
	return C.CString(r.MatchFeedback[i].Target)
}

func main() {}
