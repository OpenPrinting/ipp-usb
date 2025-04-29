/* Go IPP - IPP core protocol implementation in pure Go
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * IPP Status Codes
 */

package goipp

import (
	"fmt"
)

// Status represents an IPP Status Code
type Status Code

// Status codes
const (
	StatusOk                              Status = 0x0000 // successful-ok
	StatusOkIgnoredOrSubstituted          Status = 0x0001 // successful-ok-ignored-or-substituted-attributes
	StatusOkConflicting                   Status = 0x0002 // successful-ok-conflicting-attributes
	StatusOkIgnoredSubscriptions          Status = 0x0003 // successful-ok-ignored-subscriptions
	StatusOkIgnoredNotifications          Status = 0x0004 // successful-ok-ignored-notifications
	StatusOkTooManyEvents                 Status = 0x0005 // successful-ok-too-many-events
	StatusOkButCancelSubscription         Status = 0x0006 // successful-ok-but-cancel-subscription
	StatusOkEventsComplete                Status = 0x0007 // successful-ok-events-complete
	StatusRedirectionOtherSite            Status = 0x0200 // redirection-other-site
	StatusCupsSeeOther                    Status = 0x0280 // cups-see-other
	StatusErrorBadRequest                 Status = 0x0400 // client-error-bad-request
	StatusErrorForbidden                  Status = 0x0401 // client-error-forbidden
	StatusErrorNotAuthenticated           Status = 0x0402 // client-error-not-authenticated
	StatusErrorNotAuthorized              Status = 0x0403 // client-error-not-authorized
	StatusErrorNotPossible                Status = 0x0404 // client-error-not-possible
	StatusErrorTimeout                    Status = 0x0405 // client-error-timeout
	StatusErrorNotFound                   Status = 0x0406 // client-error-not-found
	StatusErrorGone                       Status = 0x0407 // client-error-gone
	StatusErrorRequestEntity              Status = 0x0408 // client-error-request-entity-too-large
	StatusErrorRequestValue               Status = 0x0409 // client-error-request-value-too-long
	StatusErrorDocumentFormatNotSupported Status = 0x040a // client-error-document-format-not-supported
	StatusErrorAttributesOrValues         Status = 0x040b // client-error-attributes-or-values-not-supported
	StatusErrorURIScheme                  Status = 0x040c // client-error-uri-scheme-not-supported
	StatusErrorCharset                    Status = 0x040d // client-error-charset-not-supported
	StatusErrorConflicting                Status = 0x040e // client-error-conflicting-attributes
	StatusErrorCompressionNotSupported    Status = 0x040f // client-error-compression-not-supported
	StatusErrorCompressionError           Status = 0x0410 // client-error-compression-error
	StatusErrorDocumentFormatError        Status = 0x0411 // client-error-document-format-error
	StatusErrorDocumentAccess             Status = 0x0412 // client-error-document-access-error
	StatusErrorAttributesNotSettable      Status = 0x0413 // client-error-attributes-not-settable
	StatusErrorIgnoredAllSubscriptions    Status = 0x0414 // client-error-ignored-all-subscriptions
	StatusErrorTooManySubscriptions       Status = 0x0415 // client-error-too-many-subscriptions
	StatusErrorIgnoredAllNotifications    Status = 0x0416 // client-error-ignored-all-notifications
	StatusErrorPrintSupportFileNotFound   Status = 0x0417 // client-error-print-support-file-not-found
	StatusErrorDocumentPassword           Status = 0x0418 // client-error-document-password-error
	StatusErrorDocumentPermission         Status = 0x0419 // client-error-document-permission-error
	StatusErrorDocumentSecurity           Status = 0x041a // client-error-document-security-error
	StatusErrorDocumentUnprintable        Status = 0x041b // client-error-document-unprintable-error
	StatusErrorAccountInfoNeeded          Status = 0x041c // client-error-account-info-needed
	StatusErrorAccountClosed              Status = 0x041d // client-error-account-closed
	StatusErrorAccountLimitReached        Status = 0x041e // client-error-account-limit-reached
	StatusErrorAccountAuthorizationFailed Status = 0x041f // client-error-account-authorization-failed
	StatusErrorNotFetchable               Status = 0x0420 // client-error-not-fetchable
	StatusErrorInternal                   Status = 0x0500 // server-error-internal-error
	StatusErrorOperationNotSupported      Status = 0x0501 // server-error-operation-not-supported
	StatusErrorServiceUnavailable         Status = 0x0502 // server-error-service-unavailable
	StatusErrorVersionNotSupported        Status = 0x0503 // server-error-version-not-supported
	StatusErrorDevice                     Status = 0x0504 // server-error-device-error
	StatusErrorTemporary                  Status = 0x0505 // server-error-temporary-error
	StatusErrorNotAcceptingJobs           Status = 0x0506 // server-error-not-accepting-jobs
	StatusErrorBusy                       Status = 0x0507 // server-error-busy
	StatusErrorJobCanceled                Status = 0x0508 // server-error-job-canceled
	StatusErrorMultipleJobsNotSupported   Status = 0x0509 // server-error-multiple-document-jobs-not-supported
	StatusErrorPrinterIsDeactivated       Status = 0x050a // server-error-printer-is-deactivated
	StatusErrorTooManyJobs                Status = 0x050b // server-error-too-many-jobs
	StatusErrorTooManyDocuments           Status = 0x050c // server-error-too-many-documents
)

// String() returns a Status name, as defined by RFC 8010
func (status Status) String() string {
	if s := statusNames[status]; s != "" {
		return s
	}

	return fmt.Sprintf("0x%4.4x", int(status))
}

var statusNames = map[Status]string{
	StatusOk:                              "successful-ok",
	StatusOkIgnoredOrSubstituted:          "successful-ok-ignored-or-substituted-attributes",
	StatusOkConflicting:                   "successful-ok-conflicting-attributes",
	StatusOkIgnoredSubscriptions:          "successful-ok-ignored-subscriptions",
	StatusOkIgnoredNotifications:          "successful-ok-ignored-notifications",
	StatusOkTooManyEvents:                 "successful-ok-too-many-events",
	StatusOkButCancelSubscription:         "successful-ok-but-cancel-subscription",
	StatusOkEventsComplete:                "successful-ok-events-complete",
	StatusRedirectionOtherSite:            "redirection-other-site",
	StatusCupsSeeOther:                    "cups-see-other",
	StatusErrorBadRequest:                 "client-error-bad-request",
	StatusErrorForbidden:                  "client-error-forbidden",
	StatusErrorNotAuthenticated:           "client-error-not-authenticated",
	StatusErrorNotAuthorized:              "client-error-not-authorized",
	StatusErrorNotPossible:                "client-error-not-possible",
	StatusErrorTimeout:                    "client-error-timeout",
	StatusErrorNotFound:                   "client-error-not-found",
	StatusErrorGone:                       "client-error-gone",
	StatusErrorRequestEntity:              "client-error-request-entity-too-large",
	StatusErrorRequestValue:               "client-error-request-value-too-long",
	StatusErrorDocumentFormatNotSupported: "client-error-document-format-not-supported",
	StatusErrorAttributesOrValues:         "client-error-attributes-or-values-not-supported",
	StatusErrorURIScheme:                  "client-error-uri-scheme-not-supported",
	StatusErrorCharset:                    "client-error-charset-not-supported",
	StatusErrorConflicting:                "client-error-conflicting-attributes",
	StatusErrorCompressionNotSupported:    "client-error-compression-not-supported",
	StatusErrorCompressionError:           "client-error-compression-error",
	StatusErrorDocumentFormatError:        "client-error-document-format-error",
	StatusErrorDocumentAccess:             "client-error-document-access-error",
	StatusErrorAttributesNotSettable:      "client-error-attributes-not-settable",
	StatusErrorIgnoredAllSubscriptions:    "client-error-ignored-all-subscriptions",
	StatusErrorTooManySubscriptions:       "client-error-too-many-subscriptions",
	StatusErrorIgnoredAllNotifications:    "client-error-ignored-all-notifications",
	StatusErrorPrintSupportFileNotFound:   "client-error-print-support-file-not-found",
	StatusErrorDocumentPassword:           "client-error-document-password-error",
	StatusErrorDocumentPermission:         "client-error-document-permission-error",
	StatusErrorDocumentSecurity:           "client-error-document-security-error",
	StatusErrorDocumentUnprintable:        "client-error-document-unprintable-error",
	StatusErrorAccountInfoNeeded:          "client-error-account-info-needed",
	StatusErrorAccountClosed:              "client-error-account-closed",
	StatusErrorAccountLimitReached:        "client-error-account-limit-reached",
	StatusErrorAccountAuthorizationFailed: "client-error-account-authorization-failed",
	StatusErrorNotFetchable:               "client-error-not-fetchable",
	StatusErrorInternal:                   "server-error-internal-error",
	StatusErrorOperationNotSupported:      "server-error-operation-not-supported",
	StatusErrorServiceUnavailable:         "server-error-service-unavailable",
	StatusErrorVersionNotSupported:        "server-error-version-not-supported",
	StatusErrorDevice:                     "server-error-device-error",
	StatusErrorTemporary:                  "server-error-temporary-error",
	StatusErrorNotAcceptingJobs:           "server-error-not-accepting-jobs",
	StatusErrorBusy:                       "server-error-busy",
	StatusErrorJobCanceled:                "server-error-job-canceled",
	StatusErrorMultipleJobsNotSupported:   "server-error-multiple-document-jobs-not-supported",
	StatusErrorPrinterIsDeactivated:       "server-error-printer-is-deactivated",
	StatusErrorTooManyJobs:                "server-error-too-many-jobs",
	StatusErrorTooManyDocuments:           "server-error-too-many-documents",
}
