package tron

func blockTimestampSeconds(timestamp int64) uint64 {
	if timestamp <= 0 {
		return 0
	}
	if timestamp > 1_000_000_000_000 {
		return uint64(timestamp / 1000)
	}
	return uint64(timestamp)
}
