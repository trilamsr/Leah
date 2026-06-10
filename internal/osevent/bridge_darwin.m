// macOS push-notification pump. Single CFRunLoop hosts every observer; cgo
// callbacks marshal events back into Go. ARC-managed (see CFLAGS -fobjc-arc).

#import <Cocoa/Cocoa.h>
#import <Contacts/Contacts.h>
#include <stdint.h>

// Forward declaration of the Go-exported sink.
extern void leahOSEventDeliver(uintptr_t handle, char* kind, char* bundleID, char* name);

static uintptr_t gHandle = 0;

static const char *kWorkspaceActiveApp  = "workspace.active_app_changed";
static const char *kWorkspaceLaunched   = "workspace.launched";
static const char *kWorkspaceTerminated = "workspace.terminated";
static const char *kWorkspaceWillSleep  = "workspace.will_sleep";
static const char *kWorkspaceDidWake    = "workspace.did_wake";
static const char *kContactStoreChanged = "contact_store_changed";

static void deliver(const char *kind, NSRunningApplication *app) {
    if (gHandle == 0) return;
    const char *bundle = "";
    const char *name = "";
    if (app != nil) {
        if (app.bundleIdentifier != nil) bundle = [app.bundleIdentifier UTF8String];
        if (app.localizedName != nil) name = [app.localizedName UTF8String];
    }
    leahOSEventDeliver(gHandle, (char*)kind, (char*)bundle, (char*)name);
}

void leahOSEventStart(uintptr_t handle) {
    gHandle = handle;

    NSNotificationCenter *wsCenter = [[NSWorkspace sharedWorkspace] notificationCenter];

    [wsCenter addObserverForName:NSWorkspaceDidActivateApplicationNotification
                          object:nil
                           queue:nil
                      usingBlock:^(NSNotification *note) {
        NSRunningApplication *app = note.userInfo[NSWorkspaceApplicationKey];
        deliver(kWorkspaceActiveApp, app);
    }];

    [wsCenter addObserverForName:NSWorkspaceDidLaunchApplicationNotification
                          object:nil
                           queue:nil
                      usingBlock:^(NSNotification *note) {
        NSRunningApplication *app = note.userInfo[NSWorkspaceApplicationKey];
        deliver(kWorkspaceLaunched, app);
    }];

    [wsCenter addObserverForName:NSWorkspaceDidTerminateApplicationNotification
                          object:nil
                           queue:nil
                      usingBlock:^(NSNotification *note) {
        NSRunningApplication *app = note.userInfo[NSWorkspaceApplicationKey];
        deliver(kWorkspaceTerminated, app);
    }];

    [wsCenter addObserverForName:NSWorkspaceWillSleepNotification
                          object:nil
                           queue:nil
                      usingBlock:^(NSNotification *note) {
        deliver(kWorkspaceWillSleep, nil);
    }];

    [wsCenter addObserverForName:NSWorkspaceDidWakeNotification
                          object:nil
                           queue:nil
                      usingBlock:^(NSNotification *note) {
        deliver(kWorkspaceDidWake, nil);
    }];

    // Contacts: CNContactStoreDidChangeNotification posts on the default center,
    // NOT the workspace center.
    [[NSNotificationCenter defaultCenter]
        addObserverForName:CNContactStoreDidChangeNotification
                    object:nil
                     queue:nil
                usingBlock:^(NSNotification *note) {
        if (gHandle == 0) return;
        leahOSEventDeliver(gHandle, (char*)kContactStoreChanged, (char*)"", (char*)"");
    }];

    // dispatch_main blocks forever pumping the main queue + RunLoop sources.
    // The Go side spawns this on a locked OS thread, so we own that thread.
    [[NSRunLoop currentRunLoop] run];
}

void leahOSEventStop(void) {
    // NSWorkspace blocks retained by the notification center are released when
    // the process exits; per-process pump is the documented pattern. Stop is a
    // no-op for the singleton lifetime — kept for symmetry with Source.Close.
    gHandle = 0;
}
