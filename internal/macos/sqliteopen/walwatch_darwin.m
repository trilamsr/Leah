// FSEvents-backed WAL watcher. One stream per watcher instance; runs on a
// dedicated dispatch queue so the FSEvents callback never blocks the caller.
// Watches the parent dir of the supplied path — SQLite WAL truncation can
// recreate the file inode, and a file-scoped watch would silently stop firing
// after the first checkpoint.

#import <CoreServices/CoreServices.h>
#include <stdint.h>
#include <string.h>

extern void leahWALWatchDeliver(uintptr_t handle);

typedef struct {
    uintptr_t handle;
    FSEventStreamRef stream;
    dispatch_queue_t queue;
} leah_wal_stream;

#define LEAH_WAL_MAX 16
static leah_wal_stream gStreams[LEAH_WAL_MAX];
static int gNext = 0;
static dispatch_once_t gLockOnce;
static dispatch_semaphore_t gLock = NULL;

static void ensureLock(void) {
    dispatch_once(&gLockOnce, ^{
        gLock = dispatch_semaphore_create(1);
    });
}

static void fsCallback(
    ConstFSEventStreamRef stream,
    void *clientInfo,
    size_t numEvents,
    void *eventPaths,
    const FSEventStreamEventFlags *flags,
    const FSEventStreamEventId *ids) {
    (void)stream; (void)numEvents; (void)eventPaths; (void)flags; (void)ids;
    uintptr_t h = (uintptr_t)clientInfo;
    leahWALWatchDeliver(h);
}

// Parent dir of path — FSEvents is dir-scoped.
static CFStringRef parentDir(const char *path) {
    if (path == NULL) return NULL;
    const char *slash = strrchr(path, '/');
    if (slash == NULL) return CFStringCreateWithCString(NULL, ".", kCFStringEncodingUTF8);
    size_t n = (size_t)(slash - path);
    if (n == 0) return CFStringCreateWithCString(NULL, "/", kCFStringEncodingUTF8);
    char buf[1024];
    if (n >= sizeof(buf)) return NULL;
    memcpy(buf, path, n);
    buf[n] = 0;
    return CFStringCreateWithCString(NULL, buf, kCFStringEncodingUTF8);
}

int leahWALWatchStart(uintptr_t handle, const char *path) {
    ensureLock();
    CFStringRef dir = parentDir(path);
    if (dir == NULL) return -1;
    CFArrayRef paths = CFArrayCreate(NULL, (const void **)&dir, 1, &kCFTypeArrayCallBacks);

    FSEventStreamContext ctx = {0};
    ctx.info = (void *)handle;
    // 50ms coalesce window at the kernel level — Go-side debounce adds the
    // semantic dedupe on top (same source firing for multiple consumers).
    FSEventStreamRef s = FSEventStreamCreate(
        NULL, &fsCallback, &ctx, paths,
        kFSEventStreamEventIdSinceNow, 0.05,
        kFSEventStreamCreateFlagFileEvents);
    CFRelease(paths);
    CFRelease(dir);
    if (s == NULL) return -1;

    // Reserve the slot BEFORE creating the dispatch_queue / starting the
    // stream — failure paths after queue creation would otherwise leak the
    // queue under ARC (FSEventStreamSetDispatchQueue takes a retain we cannot
    // drop via dispatch_release).
    dispatch_semaphore_wait(gLock, DISPATCH_TIME_FOREVER);
    int slot = -1;
    for (int i = 0; i < LEAH_WAL_MAX; i++) {
        int j = (gNext + i) % LEAH_WAL_MAX;
        if (gStreams[j].stream == NULL) { slot = j; break; }
    }
    if (slot < 0) {
        dispatch_semaphore_signal(gLock);
        FSEventStreamInvalidate(s);
        FSEventStreamRelease(s);
        return -1;
    }

    dispatch_queue_t q = dispatch_queue_create("com.leah.walwatch", DISPATCH_QUEUE_SERIAL);
    FSEventStreamSetDispatchQueue(s, q);
    if (!FSEventStreamStart(s)) {
        dispatch_semaphore_signal(gLock);
        FSEventStreamInvalidate(s);
        FSEventStreamRelease(s);
        return -1;
    }
    gStreams[slot].handle = handle;
    gStreams[slot].stream = s;
    gStreams[slot].queue = q;
    gNext = (slot + 1) % LEAH_WAL_MAX;
    dispatch_semaphore_signal(gLock);
    return slot;
}

void leahWALWatchStop(int streamID) {
    ensureLock();
    if (streamID < 0 || streamID >= LEAH_WAL_MAX) return;
    dispatch_semaphore_wait(gLock, DISPATCH_TIME_FOREVER);
    FSEventStreamRef s = gStreams[streamID].stream;
    gStreams[streamID].stream = NULL;
    gStreams[streamID].queue = NULL;
    gStreams[streamID].handle = 0;
    dispatch_semaphore_signal(gLock);
    if (s != NULL) {
        FSEventStreamStop(s);
        FSEventStreamInvalidate(s);
        FSEventStreamRelease(s);
    }
}
