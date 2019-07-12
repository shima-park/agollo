package agollo

type ChangeType string

const (
	ChangeTypeAdd    ChangeType = "add"
	ChangeTypeUpdate ChangeType = "update"
	ChangeTypeDelete ChangeType = "delete"
)

type Change struct {
	Type  ChangeType
	Key   string
	Value interface{}
}

type Changes []Change

// Len is part of sort.Interface.
func (cs Changes) Len() int {
	return len(cs)
}

// Swap is part of sort.Interface.
func (cs Changes) Swap(i, j int) {
	cs[i], cs[j] = cs[j], cs[i]
}

// Less is part of sort.Interface. It is implemented by calling the "by" closure in the sorter.
func (cs Changes) Less(i, j int) bool {
	return cs[i].Key < cs[j].Key
}
