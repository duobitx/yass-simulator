package model

type FsNodeState struct {
	Online bool
	IP     string
	Lat    float32
	Lng    float32
	Alt    float32
	PosX   float32
	PosY   float32
	PosZ   float32
}
