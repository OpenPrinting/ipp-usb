/* Go IPP - IPP core protocol implementation in pure Go
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * Groups of attributes
 */

package goipp

import "sort"

// Group represents a group of attributes.
//
// Since 1.1.0
type Group struct {
	Tag   Tag        // Group tag
	Attrs Attributes // Group attributes
}

// Groups represents a sequence of groups
//
// The primary purpose of this type is to represent
// messages with repeated groups with the same group tag
//
// # See Message type documentation for more details
//
// Since 1.1.0
type Groups []Group

// Add Attribute to the Group
func (g *Group) Add(attr Attribute) {
	g.Attrs.Add(attr)
}

// Equal checks that groups g and g2 are equal
func (g Group) Equal(g2 Group) bool {
	return g.Tag == g2.Tag && g.Attrs.Equal(g2.Attrs)
}

// Similar checks that groups g and g2 are **logically** equal.
func (g Group) Similar(g2 Group) bool {
	return g.Tag == g2.Tag && g.Attrs.Similar(g2.Attrs)
}

// Clone creates a shallow copy of the Group
func (g Group) Clone() Group {
	g2 := g
	g2.Attrs = g.Attrs.Clone()
	return g2
}

// DeepCopy creates a deep copy of the Group
func (g Group) DeepCopy() Group {
	g2 := g
	g2.Attrs = g.Attrs.DeepCopy()
	return g2
}

// Add Group to Groups
func (groups *Groups) Add(g Group) {
	*groups = append(*groups, g)
}

// Clone creates a shallow copy of Groups.
// For nil input it returns nil output.
func (groups Groups) Clone() Groups {
	var groups2 Groups
	if groups != nil {
		groups2 = make(Groups, len(groups))
		copy(groups2, groups)
	}
	return groups2
}

// DeepCopy creates a deep copy of Groups.
// For nil input it returns nil output.
func (groups Groups) DeepCopy() Groups {
	var groups2 Groups
	if groups != nil {
		groups2 = make(Groups, len(groups))
		for i := range groups {
			groups2[i] = groups[i].DeepCopy()
		}
	}
	return groups2
}

// Equal checks that groups and groups2 are equal.
//
// Note, Groups(nil) and Groups{} are not equal but similar.
func (groups Groups) Equal(groups2 Groups) bool {
	if len(groups) != len(groups2) {
		return false
	}

	if (groups == nil) != (groups2 == nil) {
		return false
	}

	for i, g := range groups {
		g2 := groups2[i]
		if !g.Equal(g2) {
			return false
		}
	}

	return true
}

// Similar checks that groups and groups2 are **logically** equal,
// which means the following:
//   - groups and groups2 contain the same set of
//     groups, but groups with different tags may
//     be reordered between each other.
//   - groups with the same tag cannot be reordered.
//   - attributes of corresponding groups are similar.
//
// Note, Groups(nil) and Groups{} are not equal but similar.
func (groups Groups) Similar(groups2 Groups) bool {
	// Fast check: if lengths are not the same, groups
	// are definitely not equal
	if len(groups) != len(groups2) {
		return false
	}

	// Sort groups by tag
	groups = groups.Clone()
	groups2 = groups2.Clone()

	sort.SliceStable(groups, func(i, j int) bool {
		return groups[i].Tag < groups[j].Tag
	})

	sort.SliceStable(groups2, func(i, j int) bool {
		return groups2[i].Tag < groups2[j].Tag
	})

	// Now compare, group by group
	for i := range groups {
		if !groups[i].Similar(groups2[i]) {
			return false
		}
	}

	return true
}
