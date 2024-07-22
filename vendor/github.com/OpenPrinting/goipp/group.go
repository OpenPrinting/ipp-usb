/* Go IPP - IPP core protocol implementation in pure Go
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * Groups of attributes
 */

package goipp

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
// See Message type documentation for more details
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

// Add Group to Groups
func (groups *Groups) Add(g Group) {
	*groups = append(*groups, g)
}

// Equal checks that groups and groups2 are equal
func (groups Groups) Equal(groups2 Groups) bool {
	if len(groups) != len(groups2) {
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
