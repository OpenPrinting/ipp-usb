/* Go IPP - IPP core protocol implementation in pure Go
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * IPP Tags
 */

package goipp

import (
	"fmt"
)

// Tag represents a tag used in a binary representation
// of the IPP message
type Tag int

// Tag values
const (
	// Delimiter tags
	TagZero                   Tag = 0x00 // Zero tag - used for separators
	TagOperationGroup         Tag = 0x01 // Operation group
	TagJobGroup               Tag = 0x02 // Job group
	TagEnd                    Tag = 0x03 // End-of-attributes
	TagPrinterGroup           Tag = 0x04 // Printer group
	TagUnsupportedGroup       Tag = 0x05 // Unsupported attributes group
	TagSubscriptionGroup      Tag = 0x06 // Subscription group
	TagEventNotificationGroup Tag = 0x07 // Event group
	TagResourceGroup          Tag = 0x08 // Resource group
	TagDocumentGroup          Tag = 0x09 // Document group
	TagSystemGroup            Tag = 0x0a // System group
	TagFuture11Group          Tag = 0x0b // Future group 11
	TagFuture12Group          Tag = 0x0c // Future group 12
	TagFuture13Group          Tag = 0x0d // Future group 13
	TagFuture14Group          Tag = 0x0e // Future group 14
	TagFuture15Group          Tag = 0x0f // Future group 15

	// Value tags
	TagUnsupportedValue Tag = 0x10 // Unsupported value
	TagDefault          Tag = 0x11 // Default value
	TagUnknown          Tag = 0x12 // Unknown value
	TagNoValue          Tag = 0x13 // No-value value
	TagNotSettable      Tag = 0x15 // Not-settable value
	TagDeleteAttr       Tag = 0x16 // Delete-attribute value
	TagAdminDefine      Tag = 0x17 // Admin-defined value
	TagInteger          Tag = 0x21 // Integer value
	TagBoolean          Tag = 0x22 // Boolean value
	TagEnum             Tag = 0x23 // Enumeration value
	TagString           Tag = 0x30 // Octet string value
	TagDateTime         Tag = 0x31 // Date/time value
	TagResolution       Tag = 0x32 // Resolution value
	TagRange            Tag = 0x33 // Range value
	TagBeginCollection  Tag = 0x34 // Beginning of collection value
	TagTextLang         Tag = 0x35 // Text-with-language value
	TagNameLang         Tag = 0x36 // Name-with-language value
	TagEndCollection    Tag = 0x37 // End of collection value
	TagText             Tag = 0x41 // Text value
	TagName             Tag = 0x42 // Name value
	TagReservedString   Tag = 0x43 // Reserved for future string value
	TagKeyword          Tag = 0x44 // Keyword value
	TagURI              Tag = 0x45 // URI value
	TagURIScheme        Tag = 0x46 // URI scheme value
	TagCharset          Tag = 0x47 // Character set value
	TagLanguage         Tag = 0x48 // Language value
	TagMimeType         Tag = 0x49 // MIME media type value
	TagMemberName       Tag = 0x4a // Collection member name value
	TagExtension        Tag = 0x7f // Extension point for 32-bit tags
)

// IsDelimiter returns true for delimiter tags
func (tag Tag) IsDelimiter() bool {
	return uint(tag) < 0x10
}

// IsGroup returns true for group tags
func (tag Tag) IsGroup() bool {
	return tag.IsDelimiter() && tag != TagZero && tag != TagEnd
}

// Type returns Type of Value that corresponds to the tag
func (tag Tag) Type() Type {
	if tag.IsDelimiter() {
		return TypeInvalid
	}

	switch tag {
	case TagInteger, TagEnum:
		return TypeInteger

	case TagBoolean:
		return TypeBoolean

	case TagUnsupportedValue, TagDefault, TagUnknown, TagNotSettable,
		TagDeleteAttr, TagAdminDefine:
		// These tags not expected to have value
		return TypeVoid

	case TagText, TagName, TagReservedString, TagKeyword, TagURI, TagURIScheme,
		TagCharset, TagLanguage, TagMimeType, TagMemberName:
		return TypeString

	case TagDateTime:
		return TypeDateTime

	case TagResolution:
		return TypeResolution

	case TagRange:
		return TypeRange

	case TagTextLang, TagNameLang:
		return TypeTextWithLang

	case TagBeginCollection:
		return TypeCollection

	case TagEndCollection:
		return TypeVoid

	default:
		return TypeBinary
	}
}

// String() returns a tag name, as defined by RFC 8010
func (tag Tag) String() string {
	if 0 <= tag && int(tag) < len(tagNames) {
		if s := tagNames[tag]; s != "" {
			return s
		}
	}

	if tag < 0x100 {
		return fmt.Sprintf("0x%2.2x", uint(tag))
	}

	return fmt.Sprintf("0x%8.8x", uint(tag))
}

var tagNames = [...]string{
	// Delimiter tags
	TagZero:                   "zero",
	TagOperationGroup:         "operation-attributes-tag",
	TagJobGroup:               "job-attributes-tag",
	TagEnd:                    "end-of-attributes-tag",
	TagPrinterGroup:           "printer-attributes-tag",
	TagUnsupportedGroup:       "unsupported-attributes-tag",
	TagSubscriptionGroup:      "subscription-attributes-tag",
	TagEventNotificationGroup: "event-notification-attributes-tag",
	TagResourceGroup:          "resource-attributes-tag",
	TagDocumentGroup:          "document-attributes-tag",
	TagSystemGroup:            "system-attributes-tag",

	// Value tags
	TagUnsupportedValue: "unsupported",
	TagDefault:          "default",
	TagUnknown:          "unknown",
	TagNoValue:          "no-value",
	TagNotSettable:      "not-settable",
	TagDeleteAttr:       "delete-attribute",
	TagAdminDefine:      "admin-define",
	TagInteger:          "integer",
	TagBoolean:          "boolean",
	TagEnum:             "enum",
	TagString:           "octetString",
	TagDateTime:         "dateTime",
	TagResolution:       "resolution",
	TagRange:            "rangeOfInteger",
	TagBeginCollection:  "collection",
	TagTextLang:         "textWithLanguage",
	TagNameLang:         "nameWithLanguage",
	TagEndCollection:    "endCollection",
	TagText:             "textWithoutLanguage",
	TagName:             "nameWithoutLanguage",
	TagKeyword:          "keyword",
	TagURI:              "uri",
	TagURIScheme:        "uriScheme",
	TagCharset:          "charset",
	TagLanguage:         "naturalLanguage",
	TagMimeType:         "mimeMediaType",
	TagMemberName:       "memberAttrName",
}
