package model

type SharedNodeInfo struct {
	IP         string            `json:"ip"`
	Online     bool              `json:"online"`
	Experiment string            `json:"experiment"`
	NodeType   string            `json:"node_type"`
	Properties map[string]string `json:"properties"`
}

func (i SharedNodeInfo) Eq(entry SharedNodeInfo) bool {
	return i.Online == entry.Online && i.IP == entry.IP && i.Experiment == entry.Experiment
}
