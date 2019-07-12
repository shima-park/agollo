package agollo

import "sort"

type Configurations map[string]interface{}

func (old Configurations) Different(new Configurations) Changes {
	var changes []Change
	for k, newValue := range new {
		oldValue, found := old[k]
		if found {
			if oldValue != newValue {
				changes = append(changes, Change{
					Type:  ChangeTypeUpdate,
					Key:   k,
					Value: newValue,
				})
			}
		} else {
			changes = append(changes, Change{
				Type:  ChangeTypeAdd,
				Key:   k,
				Value: newValue,
			})
		}
	}

	for k, oldValue := range old {
		_, found := new[k]
		if !found {
			changes = append(changes, Change{
				Type:  ChangeTypeDelete,
				Key:   k,
				Value: oldValue,
			})
		}
	}

	sort.Sort(Changes(changes))

	return changes
}
