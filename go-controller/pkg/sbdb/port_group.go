// Code generated by "libovsdb.modelgen"
// DO NOT EDIT.

package sbdb

import "github.com/ovn-kubernetes/libovsdb/model"

const PortGroupTable = "Port_Group"

// PortGroup defines an object in Port_Group table
type PortGroup struct {
	UUID  string   `ovsdb:"_uuid"`
	Name  string   `ovsdb:"name"`
	Ports []string `ovsdb:"ports"`
}

func (a *PortGroup) GetUUID() string {
	return a.UUID
}

func (a *PortGroup) GetName() string {
	return a.Name
}

func (a *PortGroup) GetPorts() []string {
	return a.Ports
}

func copyPortGroupPorts(a []string) []string {
	if a == nil {
		return nil
	}
	b := make([]string, len(a))
	copy(b, a)
	return b
}

func equalPortGroupPorts(a, b []string) bool {
	if (a == nil) != (b == nil) {
		return false
	}
	if len(a) != len(b) {
		return false
	}
	for i, v := range a {
		if b[i] != v {
			return false
		}
	}
	return true
}

func (a *PortGroup) DeepCopyInto(b *PortGroup) {
	*b = *a
	b.Ports = copyPortGroupPorts(a.Ports)
}

func (a *PortGroup) DeepCopy() *PortGroup {
	b := new(PortGroup)
	a.DeepCopyInto(b)
	return b
}

func (a *PortGroup) CloneModelInto(b model.Model) {
	c := b.(*PortGroup)
	a.DeepCopyInto(c)
}

func (a *PortGroup) CloneModel() model.Model {
	return a.DeepCopy()
}

func (a *PortGroup) Equals(b *PortGroup) bool {
	return a.UUID == b.UUID &&
		a.Name == b.Name &&
		equalPortGroupPorts(a.Ports, b.Ports)
}

func (a *PortGroup) EqualsModel(b model.Model) bool {
	c := b.(*PortGroup)
	return a.Equals(c)
}

var _ model.CloneableModel = &PortGroup{}
var _ model.ComparableModel = &PortGroup{}
