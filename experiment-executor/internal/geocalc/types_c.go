package geocalc

const maxSat = 200

type satPosDescr struct {
	Name   [32]byte
	X      float64
	Y      float64
	Z      float64
	Lat    float64
	Lng    float64
	Alt    float64
	NRef   int32
	SatRef [maxSat]satRefDscr
}

type satRefDscr struct {
	Sid  int32
	Dist float32
}

type common struct {
	Busy    int32
	Nsat    int32
	Nbs     int32
	UtcDttm [32]byte
	Sats    [maxSat]satPosDescr
}
