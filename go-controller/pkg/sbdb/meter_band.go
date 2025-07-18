// Code generated by "libovsdb.modelgen"
// DO NOT EDIT.

package sbdb

import "github.com/ovn-kubernetes/libovsdb/model"

const MeterBandTable = "Meter_Band"

type (
	MeterBandAction = string
)

var (
	MeterBandActionDrop MeterBandAction = "drop"
)

// MeterBand defines an object in Meter_Band table
type MeterBand struct {
	UUID      string          `ovsdb:"_uuid"`
	Action    MeterBandAction `ovsdb:"action"`
	BurstSize int             `ovsdb:"burst_size"`
	Rate      int             `ovsdb:"rate"`
}

func (a *MeterBand) GetUUID() string {
	return a.UUID
}

func (a *MeterBand) GetAction() MeterBandAction {
	return a.Action
}

func (a *MeterBand) GetBurstSize() int {
	return a.BurstSize
}

func (a *MeterBand) GetRate() int {
	return a.Rate
}

func (a *MeterBand) DeepCopyInto(b *MeterBand) {
	*b = *a
}

func (a *MeterBand) DeepCopy() *MeterBand {
	b := new(MeterBand)
	a.DeepCopyInto(b)
	return b
}

func (a *MeterBand) CloneModelInto(b model.Model) {
	c := b.(*MeterBand)
	a.DeepCopyInto(c)
}

func (a *MeterBand) CloneModel() model.Model {
	return a.DeepCopy()
}

func (a *MeterBand) Equals(b *MeterBand) bool {
	return a.UUID == b.UUID &&
		a.Action == b.Action &&
		a.BurstSize == b.BurstSize &&
		a.Rate == b.Rate
}

func (a *MeterBand) EqualsModel(b model.Model) bool {
	c := b.(*MeterBand)
	return a.Equals(c)
}

var _ model.CloneableModel = &MeterBand{}
var _ model.ComparableModel = &MeterBand{}
