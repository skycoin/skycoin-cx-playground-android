package netbatch

import "net/netip"

// netTimerSlotCount is the number of slots of the timer wheel, one
// millisecond each. A timer waits in the slot of its deadline modulo the
// count and is checked once per turn of the wheel.
const netTimerSlotCount = 256

// NetTimerWheel holds one timeout per handle in a hashed timer wheel
// (Varghese and Lauck, SOSP 1987). Start and stop cost O(1), and an advance
// costs the slots it passes, at most one turn, plus the timers in them.
type NetTimerWheel struct {
	timerDeadline [NetSocketCountMax]int64
	timerNext     [NetSocketCountMax]int32
	timerPrevious [NetSocketCountMax]int32
	timerSlotHead [netTimerSlotCount]int32
	// timerClock is the reading the wheel was last advanced to.
	timerClock int64
}

// timerWheelClear stops every timer.
func (wheel *NetTimerWheel) timerWheelClear() {
	*wheel = NetTimerWheel{timerClock: -1}
}

// TimerStart starts the timer of handle, which must not run, to expire at
// deadline, a clock reading above 0.
func (wheel *NetTimerWheel) TimerStart(handle int32, deadline int64) {
	slot := deadline % netTimerSlotCount
	wheel.timerDeadline[handle] = deadline
	wheel.timerPrevious[handle] = 0
	wheel.timerNext[handle] = wheel.timerSlotHead[slot]
	if head := wheel.timerSlotHead[slot]; head != 0 {
		wheel.timerPrevious[head] = handle
	}
	wheel.timerSlotHead[slot] = handle
}

// TimerStop stops the timer of handle if it runs.
func (wheel *NetTimerWheel) TimerStop(handle int32) {
	deadline := wheel.timerDeadline[handle]
	if deadline == 0 {
		return
	}
	next, previous := wheel.timerNext[handle], wheel.timerPrevious[handle]
	if previous != 0 {
		wheel.timerNext[previous] = next
	} else {
		wheel.timerSlotHead[deadline%netTimerSlotCount] = next
	}
	if next != 0 {
		wheel.timerPrevious[next] = previous
	}
	wheel.timerDeadline[handle] = 0
}

// TimerAdvance moves the wheel to clock and calls fire for each timer that
// expired, stopped first.
func (wheel *NetTimerWheel) TimerAdvance(clock int64, fire func(handle int32)) {
	if clock <= wheel.timerClock {
		return
	}
	steps := min(clock-wheel.timerClock, netTimerSlotCount)
	for tick := wheel.timerClock + 1; tick <= wheel.timerClock+steps; tick++ {
		handle := wheel.timerSlotHead[tick%netTimerSlotCount]
		for handle != 0 {
			next := wheel.timerNext[handle]
			if wheel.timerDeadline[handle] <= clock {
				wheel.TimerStop(handle)
				fire(handle)
			}
			handle = next
		}
	}
	wheel.timerClock = clock
}

// NetPeerTable numbers the UDP addresses a program sends to or receives
// from, in the order it first meets them, so the numbers replay. It is an
// open addressing hash table of fixed size.
type NetPeerTable struct {
	peerAddress [NetPeerCountMax]netip.AddrPort
	peerCount   int32
	peerIndex   [2 * NetPeerCountMax]int32
}

// peerTableClear empties the table.
func (table *NetPeerTable) peerTableClear() {
	*table = NetPeerTable{}
}

// peerHash is FNV-1a of the address and port.
func peerHash(address netip.AddrPort) uint32 {
	hash := uint32(2166136261)
	bytes := address.Addr().As16()
	for _, b := range bytes {
		hash = (hash ^ uint32(b)) * 16777619
	}
	hash = (hash ^ uint32(address.Port()>>8)) * 16777619
	return (hash ^ uint32(address.Port()&0xff)) * 16777619
}

// PeerFind returns the peer of address, adding it when add is set. It
// returns 0 for an address not found or a full table.
func (table *NetPeerTable) PeerFind(address netip.AddrPort, add bool) int32 {
	mask := uint32(len(table.peerIndex) - 1)
	for slot := peerHash(address) & mask; ; slot = (slot + 1) & mask {
		peer := table.peerIndex[slot]
		if peer == 0 {
			if !add || table.peerCount+1 >= NetPeerCountMax {
				return 0
			}
			table.peerCount++
			table.peerAddress[table.peerCount] = address
			table.peerIndex[slot] = table.peerCount
			return table.peerCount
		}
		if table.peerAddress[peer] == address {
			return peer
		}
	}
}

// PeerAddress returns the address of peer.
func (table *NetPeerTable) PeerAddress(peer int32) (netip.AddrPort, bool) {
	if peer <= 0 || peer > table.peerCount {
		return netip.AddrPort{}, false
	}
	return table.peerAddress[peer], true
}
