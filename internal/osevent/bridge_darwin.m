// macOS push-notification pump. Dedicated NSOperationQueue serialises observer
// callbacks; CFRunLoopSource keepalive on the pump thread guarantees the
// runloop stays alive for NSWorkspace's Mach-port delivery even if every
// observer is rebalanced onto our queue. ARC-managed (see CFLAGS -fobjc-arc).

#import <Cocoa/Cocoa.h>
#import <Contacts/Contacts.h>
#include <stdint.h>

extern void leahOSEventDeliver(uintptr_t handle, char* kind, char* bundleID, char* name);

// gHandle is written once under dispatch_once and read by every observer
// block — dispatch_once provides the publish barrier so observers never see
// a stale zero after Start.
static uintptr_t gHandle = 0;
static dispatch_once_t gStartOnce;
static NSOperationQueue *gQueue = nil;
static NSMutableArray *gObservers = nil; // strong refs to observer tokens

static const char *kWorkspaceActiveApp  = "workspace.active_app_changed";
static const char *kWorkspaceLaunched   = "workspace.launched";
static const char *kWorkspaceTerminated = "workspace.terminated";
static const char *kWorkspaceWillSleep  = "workspace.will_sleep";
static const char *kWorkspaceDidWake    = "workspace.did_wake";
static const char *kContactStoreChanged = "contact_store_changed";

static void deliver(const char *kind, NSRunningApplication *app) {
    const char *bundle = "";
    const char *name = "";
    if (app != nil) {
        if (app.bundleIdentifier != nil) bundle = [app.bundleIdentifier UTF8String];
        if (app.localizedName != nil) name = [app.localizedName UTF8String];
    }
    leahOSEventDeliver(gHandle, (char*)kind, (char*)bundle, (char*)name);
}

// Dummy CFRunLoopSource — never signalled, never fires. Its only job is to
// give the pump-thread runloop an input source so `[NSRunLoop run]` does not
// return when every observer is dispatched off-thread via gQueue.
static void noopPerform(void *info) { (void)info; }

void leahOSEventStart(uintptr_t handle) {
    dispatch_once(&gStartOnce, ^{
        gHandle = handle;
        gQueue = [[NSOperationQueue alloc] init];
        gQueue.maxConcurrentOperationCount = 1;
        gQueue.name = @"com.leah.osevent";
        gObservers = [NSMutableArray array];

        NSNotificationCenter *wsCenter = [[NSWorkspace sharedWorkspace] notificationCenter];
        NSNotificationCenter *dfCenter = [NSNotificationCenter defaultCenter];

        id t1 = [wsCenter addObserverForName:NSWorkspaceDidActivateApplicationNotification
                                      object:nil queue:gQueue
                                  usingBlock:^(NSNotification *note) {
            deliver(kWorkspaceActiveApp, note.userInfo[NSWorkspaceApplicationKey]);
        }];
        id t2 = [wsCenter addObserverForName:NSWorkspaceDidLaunchApplicationNotification
                                      object:nil queue:gQueue
                                  usingBlock:^(NSNotification *note) {
            deliver(kWorkspaceLaunched, note.userInfo[NSWorkspaceApplicationKey]);
        }];
        id t3 = [wsCenter addObserverForName:NSWorkspaceDidTerminateApplicationNotification
                                      object:nil queue:gQueue
                                  usingBlock:^(NSNotification *note) {
            deliver(kWorkspaceTerminated, note.userInfo[NSWorkspaceApplicationKey]);
        }];
        id t4 = [wsCenter addObserverForName:NSWorkspaceWillSleepNotification
                                      object:nil queue:gQueue
                                  usingBlock:^(NSNotification *note) {
            deliver(kWorkspaceWillSleep, nil);
        }];
        id t5 = [wsCenter addObserverForName:NSWorkspaceDidWakeNotification
                                      object:nil queue:gQueue
                                  usingBlock:^(NSNotification *note) {
            deliver(kWorkspaceDidWake, nil);
        }];
        // CNContactStoreDidChangeNotification posts on the default center, NOT
        // the workspace center.
        id t6 = [dfCenter addObserverForName:CNContactStoreDidChangeNotification
                                      object:nil queue:gQueue
                                  usingBlock:^(NSNotification *note) {
            leahOSEventDeliver(gHandle, (char*)kContactStoreChanged, (char*)"", (char*)"");
        }];
        [gObservers addObjectsFromArray:@[t1, t2, t3, t4, t5, t6]];
    });

    // Install a dummy CFRunLoopSource so `[NSRunLoop run]` has an input source
    // and never returns spuriously. Observers themselves fire on gQueue, off
    // this thread — the runloop is kept alive purely as a parking spot for
    // the Go-locked OS thread we were handed.
    CFRunLoopSourceContext ctx = {0};
    ctx.perform = noopPerform;
    CFRunLoopSourceRef src = CFRunLoopSourceCreate(NULL, 0, &ctx);
    CFRunLoopAddSource(CFRunLoopGetCurrent(), src, kCFRunLoopDefaultMode);
    [[NSRunLoop currentRunLoop] run];
    // Unreachable; kept for symmetry.
    CFRunLoopRemoveSource(CFRunLoopGetCurrent(), src, kCFRunLoopDefaultMode);
    CFRelease(src);
}
