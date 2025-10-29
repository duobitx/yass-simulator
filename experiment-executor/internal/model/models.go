package model

type FsNodeState struct {
	Online bool
	IP     string
}

type GeoResult struct {
	X             float32
	Y             float32
	Z             float32
	Alt           float32
	NetworkParams []GeoResultNetworkParamEntry
}

type GeoResultNetworkParamEntry struct {
	IP string
	// TODO Network params
	Delay       float32
	PackageLoss float32
}
