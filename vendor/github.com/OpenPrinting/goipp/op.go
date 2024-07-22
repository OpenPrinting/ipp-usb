/* Go IPP - IPP core protocol implementation in pure Go
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * IPP Operation Codes
 */

package goipp

import (
	"fmt"
)

// Op represents an IPP Operation Code
type Op Code

// Op codes
const (
	OpPrintJob             Op = 0x0002 // Print-Job: Print a single file
	OpPrintURI             Op = 0x0003 // Print-URI: Print a single URL
	OpValidateJob          Op = 0x0004 // Validate-Job: Validate job values prior to submission
	OpCreateJob            Op = 0x0005 // Create-Job: Create an empty print job
	OpSendDocument         Op = 0x0006 // Send-Document: Add a file to a job
	OpSendURI              Op = 0x0007 // Send-URI: Add a URL to a job
	OpCancelJob            Op = 0x0008 // Cancel-Job: Cancel a job
	OpGetJobAttributes     Op = 0x0009 // Get-Job-Attribute: Get information about a job
	OpGetJobs              Op = 0x000a // Get-Jobs: Get a list of jobs
	OpGetPrinterAttributes Op = 0x000b // Get-Printer-Attributes: Get information about a printer
	OpHoldJob              Op = 0x000c // Hold-Job: Hold a job for printing
	OpReleaseJob           Op = 0x000d // Release-Job: Release a job for printing
	OpRestartJob           Op = 0x000e // Restart-Job: Reprint a job

	OpPausePrinter               Op = 0x0010 // Pause-Printer: Stop a printer
	OpResumePrinter              Op = 0x0011 // Resume-Printer: Start a printer
	OpPurgeJobs                  Op = 0x0012 // Purge-Jobs: Delete all jobs
	OpSetPrinterAttributes       Op = 0x0013 // Set-Printer-Attributes: Set printer values
	OpSetJobAttributes           Op = 0x0014 // Set-Job-Attributes: Set job values
	OpGetPrinterSupportedValues  Op = 0x0015 // Get-Printer-Supported-Values: Get supported values
	OpCreatePrinterSubscriptions Op = 0x0016 // Create-Printer-Subscriptions: Create one or more printer subscriptions
	OpCreateJobSubscriptions     Op = 0x0017 // Create-Job-Subscriptions: Create one of more job subscriptions
	OpGetSubscriptionAttributes  Op = 0x0018 // Get-Subscription-Attributes: Get subscription information
	OpGetSubscriptions           Op = 0x0019 // Get-Subscriptions: Get list of subscriptions
	OpRenewSubscription          Op = 0x001a // Renew-Subscription: Renew a printer subscription
	OpCancelSubscription         Op = 0x001b // Cancel-Subscription: Cancel a subscription
	OpGetNotifications           Op = 0x001c // Get-Notifications: Get notification events
	OpSendNotifications          Op = 0x001d // Send-Notifications: Send notification events
	OpGetResourceAttributes      Op = 0x001e // Get-Resource-Attributes: Get resource information
	OpGetResourceData            Op = 0x001f // Get-Resource-Data: Get resource data

	OpGetResources                Op = 0x0020 // Get-Resources: Get list of resources
	OpGetPrintSupportFiles        Op = 0x0021 // Get-Printer-Support-Files: Get printer support files
	OpEnablePrinter               Op = 0x0022 // Enable-Printer: Accept new jobs for a printer
	OpDisablePrinter              Op = 0x0023 // Disable-Printer: Reject new jobs for a printer
	OpPausePrinterAfterCurrentJob Op = 0x0024 // Pause-Printer-After-Current-Job: Stop printer after the current job
	OpHoldNewJobs                 Op = 0x0025 // Hold-New-Jobs: Hold new jobs
	OpReleaseHeldNewJobs          Op = 0x0026 // Release-Held-New-Jobs: Release new jobs that were previously held
	OpDeactivatePrinter           Op = 0x0027 // Deactivate-Printer: Stop a printer and do not accept jobs
	OpActivatePrinter             Op = 0x0028 // Activate-Printer: Start a printer and accept jobs
	OpRestartPrinter              Op = 0x0029 // Restart-Printer: Restart a printer
	OpShutdownPrinter             Op = 0x002a // Shutdown-Printer: Turn a printer off
	OpStartupPrinter              Op = 0x002b // Startup-Printer: Turn a printer on
	OpReprocessJob                Op = 0x002c // Reprocess-Job: Reprint a job
	OpCancelCurrentJob            Op = 0x002d // Cancel-Current-Job: Cancel the current job
	OpSuspendCurrentJob           Op = 0x002e // Suspend-Current-Job: Suspend the current job
	OpResumeJob                   Op = 0x002f // Resume-Job: Resume the current job

	OpPromoteJob            Op = 0x0030 // Promote-Job: Promote a job to print sooner
	OpScheduleJobAfter      Op = 0x0031 // Schedule-Job-After: Schedule a job to print after another
	OpCancelDocument        Op = 0x0033 // Cancel-Document: Cancel a document
	OpGetDocumentAttributes Op = 0x0034 // Get-Document-Attributes: Get document information
	OpGetDocuments          Op = 0x0035 // Get-Documents: Get a list of documents in a job
	OpDeleteDocument        Op = 0x0036 // Delete-Document: Delete a document
	OpSetDocumentAttributes Op = 0x0037 // Set-Document-Attributes: Set document values
	OpCancelJobs            Op = 0x0038 // Cancel-Jobs: Cancel all jobs (administrative)
	OpCancelMyJobs          Op = 0x0039 // Cancel-My-Jobs: Cancel a user's jobs
	OpResubmitJob           Op = 0x003a // Resubmit-Job: Copy and reprint a job
	OpCloseJob              Op = 0x003b // Close-Job: Close a job and start printing
	OpIdentifyPrinter       Op = 0x003c // Identify-Printer: Make the printer beep, flash, or display a message for identification
	OpValidateDocument      Op = 0x003d // Validate-Document: Validate document values prior to submission
	OpAddDocumentImages     Op = 0x003e // Add-Document-Images: Add image(s) from the specified scanner source
	OpAcknowledgeDocument   Op = 0x003f // Acknowledge-Document: Acknowledge processing of a document

	OpAcknowledgeIdentifyPrinter   Op = 0x0040 // Acknowledge-Identify-Printer: Acknowledge action on an Identify-Printer request
	OpAcknowledgeJob               Op = 0x0041 // Acknowledge-Job: Acknowledge processing of a job
	OpFetchDocument                Op = 0x0042 // Fetch-Document: Fetch a document for processing
	OpFetchJob                     Op = 0x0043 // Fetch-Job: Fetch a job for processing
	OpGetOutputDeviceAttributes    Op = 0x0044 // Get-Output-Device-Attributes: Get printer information for a specific output device
	OpUpdateActiveJobs             Op = 0x0045 // Update-Active-Jobs: Update the list of active jobs that a proxy has processed
	OpDeregisterOutputDevice       Op = 0x0046 // Deregister-Output-Device: Remove an output device
	OpUpdateDocumentStatus         Op = 0x0047 // Update-Document-Status: Update document values
	OpUpdateJobStatus              Op = 0x0048 // Update-Job-Status: Update job values
	OpupdateOutputDeviceAttributes Op = 0x0049 // Update-Output-Device-Attributes: Update output device values
	OpGetNextDocumentData          Op = 0x004a // Get-Next-Document-Data: Scan more document data
	OpAllocatePrinterResources     Op = 0x004b // Allocate-Printer-Resources: Use resources for a printer
	OpCreatePrinter                Op = 0x004c // Create-Printer: Create a new service
	OpDeallocatePrinterResources   Op = 0x004d // Deallocate-Printer-Resources: Stop using resources for a printer
	OpDeletePrinter                Op = 0x004e // Delete-Printer: Delete an existing service
	OpGetPrinters                  Op = 0x004f // Get-Printers: Get a list of services

	OpShutdownOnePrinter              Op = 0x0050 // Shutdown-One-Printer: Shutdown a service
	OpStartupOnePrinter               Op = 0x0051 // Startup-One-Printer: Start a service
	OpCancelResource                  Op = 0x0052 // Cancel-Resource: Uninstall a resource
	OpCreateResource                  Op = 0x0053 // Create-Resource: Create a new (empty) resource
	OpInstallResource                 Op = 0x0054 // Install-Resource: Install a resource
	OpSendResourceData                Op = 0x0055 // Send-Resource-Data: Upload the data for a resource
	OpSetResourceAttributes           Op = 0x0056 // Set-Resource-Attributes: Set resource object  attributes
	OpCreateResourceSubscriptions     Op = 0x0057 // Create-Resource-Subscriptions: Create event subscriptions for a resource
	OpCreateSystemSubscriptions       Op = 0x0058 // Create-System-Subscriptions: Create event subscriptions for a system
	OpDisableAllPrinters              Op = 0x0059 // Disable-All-Printers: Stop accepting new jobs on all services
	OpEnableAllPrinters               Op = 0x005a // Enable-All-Printers: Start accepting new jobs on all services
	OpGetSystemAttributes             Op = 0x005b // Get-System-Attributes: Get system object attributes
	OpGetSystemSupportedValues        Op = 0x005c // Get-System-Supported-Values: Get supported values for system object attributes
	OpPauseAllPrinters                Op = 0x005d // Pause-All-Printers: Stop all services immediately
	OpPauseAllPrintersAfterCurrentJob Op = 0x005e // Pause-All-Printers-After-Current-Job: Stop all services after processing the current jobs
	OpRegisterOutputDevice            Op = 0x005f // Register-Output-Device: Register a remote service

	OpRestartSystem       Op = 0x0060 // Restart-System: Restart all services
	OpResumeAllPrinters   Op = 0x0061 // Resume-All-Printers: Start job processing on all services
	OpSetSystemAttributes Op = 0x0062 // Set-System-Attributes: Set system object attributes
	OpShutdownAllPrinters Op = 0x0063 // Shutdown-All-Printers: Shutdown all services
	OpStartupAllPrinters  Op = 0x0064 // Startup-All-Printers: Startup all services

	OpCupsGetDefault       Op = 0x4001 // CUPS-Get-Default: Get the default printer
	OpCupsGetPrinters      Op = 0x4002 // CUPS-Get-Printers: Get a list of printers and/or classes
	OpCupsAddModifyPrinter Op = 0x4003 // CUPS-Add-Modify-Printer: Add or modify a printer
	OpCupsDeletePrinter    Op = 0x4004 // CUPS-Delete-Printer: Delete a printer
	OpCupsGetClasses       Op = 0x4005 // CUPS-Get-Classes: Get a list of classes
	OpCupsAddModifyClass   Op = 0x4006 // CUPS-Add-Modify-Class: Add or modify a class
	OpCupsDeleteClass      Op = 0x4007 // CUPS-Delete-Class: Delete a class
	OpCupsAcceptJobs       Op = 0x4008 // CUPS-Accept-Jobs: Accept new jobs on a printer
	OpCupsRejectJobs       Op = 0x4009 // CUPS-Reject-Jobs: Reject new jobs on a printer
	OpCupsSetDefault       Op = 0x400a // CUPS-Set-Default: Set the default printer
	OpCupsGetDevices       Op = 0x400b // CUPS-Get-Devices: Get a list of supported devices
	OpCupsGetPpds          Op = 0x400c // CUPS-Get-PPDs: Get a list of supported drivers
	OpCupsMoveJob          Op = 0x400d // CUPS-Move-Job: Move a job to a different printer
	OpCupsAuthenticateJob  Op = 0x400e // CUPS-Authenticate-Job: Authenticate a job
	OpCupsGetPpd           Op = 0x400f // CUPS-Get-PPD: Get a PPD file

	OpCupsGetDocument        Op = 0x4027 // CUPS-Get-Document: Get a document file
	OpCupsCreateLocalPrinter Op = 0x4028 // CUPS-Create-Local-Printer: Create a local (temporary) printer

)

// String() returns a Status name, as defined by RFC 8010
func (op Op) String() string {
	if int(op) < len(opNames) {
		if s := opNames[op]; s != "" {
			return s
		}
	}

	return fmt.Sprintf("0x%4.4x", int(op))
}

var opNames = [...]string{
	OpPrintJob:                        "Print-Job",
	OpPrintURI:                        "Print-URI",
	OpValidateJob:                     "Validate-Job",
	OpCreateJob:                       "Create-Job",
	OpSendDocument:                    "Send-Document",
	OpSendURI:                         "Send-URI",
	OpCancelJob:                       "Cancel-Job",
	OpGetJobAttributes:                "Get-Job-Attribute",
	OpGetJobs:                         "Get-Jobs",
	OpGetPrinterAttributes:            "Get-Printer-Attributes",
	OpHoldJob:                         "Hold-Job",
	OpReleaseJob:                      "Release-Job",
	OpRestartJob:                      "Restart-Job",
	OpPausePrinter:                    "Pause-Printer",
	OpResumePrinter:                   "Resume-Printer",
	OpPurgeJobs:                       "Purge-Jobs",
	OpSetPrinterAttributes:            "Set-Printer-Attributes",
	OpSetJobAttributes:                "Set-Job-Attributes",
	OpGetPrinterSupportedValues:       "Get-Printer-Supported-Values",
	OpCreatePrinterSubscriptions:      "Create-Printer-Subscriptions",
	OpCreateJobSubscriptions:          "Create-Job-Subscriptions",
	OpGetSubscriptionAttributes:       "Get-Subscription-Attributes",
	OpGetSubscriptions:                "Get-Subscriptions",
	OpRenewSubscription:               "Renew-Subscription",
	OpCancelSubscription:              "Cancel-Subscription",
	OpGetNotifications:                "Get-Notifications",
	OpSendNotifications:               "Send-Notifications",
	OpGetResourceAttributes:           "Get-Resource-Attributes",
	OpGetResourceData:                 "Get-Resource-Data",
	OpGetResources:                    "Get-Resources",
	OpGetPrintSupportFiles:            "Get-Printer-Support-Files",
	OpEnablePrinter:                   "Enable-Printer",
	OpDisablePrinter:                  "Disable-Printer",
	OpPausePrinterAfterCurrentJob:     "Pause-Printer-After-Current-Job",
	OpHoldNewJobs:                     "Hold-New-Jobs",
	OpReleaseHeldNewJobs:              "Release-Held-New-Jobs",
	OpDeactivatePrinter:               "Deactivate-Printer",
	OpActivatePrinter:                 "Activate-Printer",
	OpRestartPrinter:                  "Restart-Printer",
	OpShutdownPrinter:                 "Shutdown-Printer",
	OpStartupPrinter:                  "Startup-Printer",
	OpReprocessJob:                    "Reprocess-Job",
	OpCancelCurrentJob:                "Cancel-Current-Job",
	OpSuspendCurrentJob:               "Suspend-Current-Job",
	OpResumeJob:                       "Resume-Job",
	OpPromoteJob:                      "Promote-Job",
	OpScheduleJobAfter:                "Schedule-Job-After",
	OpCancelDocument:                  "Cancel-Document",
	OpGetDocumentAttributes:           "Get-Document-Attributes",
	OpGetDocuments:                    "Get-Documents",
	OpDeleteDocument:                  "Delete-Document",
	OpSetDocumentAttributes:           "Set-Document-Attributes",
	OpCancelJobs:                      "Cancel-Jobs",
	OpCancelMyJobs:                    "Cancel-My-Jobs",
	OpResubmitJob:                     "Resubmit-Job",
	OpCloseJob:                        "Close-Job",
	OpIdentifyPrinter:                 "Identify-Printer",
	OpValidateDocument:                "Validate-Document",
	OpAddDocumentImages:               "Add-Document-Images",
	OpAcknowledgeDocument:             "Acknowledge-Document",
	OpAcknowledgeIdentifyPrinter:      "Acknowledge-Identify-Printer",
	OpAcknowledgeJob:                  "Acknowledge-Job",
	OpFetchDocument:                   "Fetch-Document",
	OpFetchJob:                        "Fetch-Job",
	OpGetOutputDeviceAttributes:       "Get-Output-Device-Attributes",
	OpUpdateActiveJobs:                "Update-Active-Jobs",
	OpDeregisterOutputDevice:          "Deregister-Output-Device",
	OpUpdateDocumentStatus:            "Update-Document-Status",
	OpUpdateJobStatus:                 "Update-Job-Status",
	OpupdateOutputDeviceAttributes:    "Update-Output-Device-Attributes",
	OpGetNextDocumentData:             "Get-Next-Document-Data",
	OpAllocatePrinterResources:        "Allocate-Printer-Resources",
	OpCreatePrinter:                   "Create-Printer",
	OpDeallocatePrinterResources:      "Deallocate-Printer-Resources",
	OpDeletePrinter:                   "Delete-Printer",
	OpGetPrinters:                     "Get-Printers",
	OpShutdownOnePrinter:              "Shutdown-One-Printer",
	OpStartupOnePrinter:               "Startup-One-Printer",
	OpCancelResource:                  "Cancel-Resource",
	OpCreateResource:                  "Create-Resource",
	OpInstallResource:                 "Install-Resource",
	OpSendResourceData:                "Send-Resource-Data",
	OpSetResourceAttributes:           "Set-Resource-Attributes",
	OpCreateResourceSubscriptions:     "Create-Resource-Subscriptions",
	OpCreateSystemSubscriptions:       "Create-System-Subscriptions",
	OpDisableAllPrinters:              "Disable-All-Printers",
	OpEnableAllPrinters:               "Enable-All-Printers",
	OpGetSystemAttributes:             "Get-System-Attributes",
	OpGetSystemSupportedValues:        "Get-System-Supported-Values",
	OpPauseAllPrinters:                "Pause-All-Printers",
	OpPauseAllPrintersAfterCurrentJob: "Pause-All-Printers-After-Current-Job",
	OpRegisterOutputDevice:            "Register-Output-Device",
	OpRestartSystem:                   "Restart-System",
	OpResumeAllPrinters:               "Resume-All-Printers",
	OpSetSystemAttributes:             "Set-System-Attributes",
	OpShutdownAllPrinters:             "Shutdown-All-Printers",
	OpStartupAllPrinters:              "Startup-All-Printers",
	OpCupsGetDefault:                  "CUPS-Get-Default",
	OpCupsGetPrinters:                 "CUPS-Get-Printers",
	OpCupsAddModifyPrinter:            "CUPS-Add-Modify-Printer",
	OpCupsDeletePrinter:               "CUPS-Delete-Printer",
	OpCupsGetClasses:                  "CUPS-Get-Classes",
	OpCupsAddModifyClass:              "CUPS-Add-Modify-Class",
	OpCupsDeleteClass:                 "CUPS-Delete-Class",
	OpCupsAcceptJobs:                  "CUPS-Accept-Jobs",
	OpCupsRejectJobs:                  "CUPS-Reject-Jobs",
	OpCupsSetDefault:                  "CUPS-Set-Default",
	OpCupsGetDevices:                  "CUPS-Get-Devices",
	OpCupsGetPpds:                     "CUPS-Get-PPDs",
	OpCupsMoveJob:                     "CUPS-Move-Job",
	OpCupsAuthenticateJob:             "CUPS-Authenticate-Job",
	OpCupsGetPpd:                      "CUPS-Get-PPD",
	OpCupsGetDocument:                 "CUPS-Get-Document",
	OpCupsCreateLocalPrinter:          "CUPS-Create-Local-Printer",
}
