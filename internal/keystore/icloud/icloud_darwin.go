//go:build darwin && cgo

package icloud

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework CoreFoundation -framework Security

#include <CoreFoundation/CoreFoundation.h>
#include <Security/Security.h>
#include <stdlib.h>
#include <string.h>

// leahSecPut writes (service, account) -> data with optional iCloud sync.
// Returns OSStatus. Uses SecItemUpdate first to make rotation atomic — a
// kill between Delete+Add would wipe the prior secret without persisting
// the new one (mirrors LeahAuth/Keychain.swift rotation guard).
static OSStatus leahSecPut(const char *svc, const char *acct,
                           const void *data, size_t dataLen,
                           int syncFlag) {
    CFStringRef cfSvc = CFStringCreateWithCString(NULL, svc, kCFStringEncodingUTF8);
    CFStringRef cfAcct = CFStringCreateWithCString(NULL, acct, kCFStringEncodingUTF8);
    CFDataRef cfData = CFDataCreate(NULL, (const UInt8 *)data, (CFIndex)dataLen);
    CFBooleanRef cfSync = syncFlag ? kCFBooleanTrue : kCFBooleanFalse;

    const void *qKeys[]   = { kSecClass, kSecAttrService, kSecAttrAccount, kSecAttrSynchronizable };
    const void *qVals[]   = { kSecClassGenericPassword, cfSvc, cfAcct, cfSync };
    CFDictionaryRef query = CFDictionaryCreate(NULL, qKeys, qVals, 4,
                                               &kCFTypeDictionaryKeyCallBacks,
                                               &kCFTypeDictionaryValueCallBacks);

    const void *uKeys[] = { kSecValueData, kSecAttrAccessible };
    const void *uVals[] = { cfData, kSecAttrAccessibleWhenUnlocked };
    CFDictionaryRef update = CFDictionaryCreate(NULL, uKeys, uVals, 2,
                                                &kCFTypeDictionaryKeyCallBacks,
                                                &kCFTypeDictionaryValueCallBacks);

    OSStatus s = SecItemUpdate(query, update);
    if (s == errSecItemNotFound) {
        const void *aKeys[] = { kSecClass, kSecAttrService, kSecAttrAccount,
                                kSecAttrSynchronizable, kSecValueData, kSecAttrAccessible };
        const void *aVals[] = { kSecClassGenericPassword, cfSvc, cfAcct,
                                cfSync, cfData, kSecAttrAccessibleWhenUnlocked };
        CFDictionaryRef add = CFDictionaryCreate(NULL, aKeys, aVals, 6,
                                                 &kCFTypeDictionaryKeyCallBacks,
                                                 &kCFTypeDictionaryValueCallBacks);
        s = SecItemAdd(add, NULL);
        CFRelease(add);
    }

    CFRelease(query);
    CFRelease(update);
    CFRelease(cfSvc);
    CFRelease(cfAcct);
    CFRelease(cfData);
    return s;
}

// leahSecGet copies the stored value into *out (caller frees with free).
// Returns OSStatus. *outLen is set on success.
static OSStatus leahSecGet(const char *svc, const char *acct, void **out, size_t *outLen) {
    *out = NULL;
    *outLen = 0;
    CFStringRef cfSvc = CFStringCreateWithCString(NULL, svc, kCFStringEncodingUTF8);
    CFStringRef cfAcct = CFStringCreateWithCString(NULL, acct, kCFStringEncodingUTF8);

    // kSecAttrSynchronizableAny matches both synced + local rows; the caller
    // is identity-keyed by (service, account) and doesn't pre-know the flag.
    const void *qKeys[] = { kSecClass, kSecAttrService, kSecAttrAccount,
                            kSecAttrSynchronizable, kSecReturnData, kSecMatchLimit };
    const void *qVals[] = { kSecClassGenericPassword, cfSvc, cfAcct,
                            kSecAttrSynchronizableAny, kCFBooleanTrue, kSecMatchLimitOne };
    CFDictionaryRef query = CFDictionaryCreate(NULL, qKeys, qVals, 6,
                                               &kCFTypeDictionaryKeyCallBacks,
                                               &kCFTypeDictionaryValueCallBacks);
    CFTypeRef raw = NULL;
    OSStatus s = SecItemCopyMatching(query, &raw);
    CFRelease(query);
    CFRelease(cfSvc);
    CFRelease(cfAcct);
    if (s != errSecSuccess) return s;
    if (CFGetTypeID(raw) != CFDataGetTypeID()) {
        CFRelease(raw);
        return errSecItemNotFound;
    }
    CFDataRef data = (CFDataRef)raw;
    CFIndex n = CFDataGetLength(data);
    void *buf = malloc((size_t)n);
    if (buf == NULL) {
        CFRelease(raw);
        return errSecAllocate;
    }
    memcpy(buf, CFDataGetBytePtr(data), (size_t)n);
    *out = buf;
    *outLen = (size_t)n;
    CFRelease(raw);
    return errSecSuccess;
}

// leahSecDelete removes the row regardless of sync flag.
static OSStatus leahSecDelete(const char *svc, const char *acct) {
    CFStringRef cfSvc = CFStringCreateWithCString(NULL, svc, kCFStringEncodingUTF8);
    CFStringRef cfAcct = CFStringCreateWithCString(NULL, acct, kCFStringEncodingUTF8);
    const void *qKeys[] = { kSecClass, kSecAttrService, kSecAttrAccount, kSecAttrSynchronizable };
    const void *qVals[] = { kSecClassGenericPassword, cfSvc, cfAcct, kSecAttrSynchronizableAny };
    CFDictionaryRef query = CFDictionaryCreate(NULL, qKeys, qVals, 4,
                                               &kCFTypeDictionaryKeyCallBacks,
                                               &kCFTypeDictionaryValueCallBacks);
    OSStatus s = SecItemDelete(query);
    CFRelease(query);
    CFRelease(cfSvc);
    CFRelease(cfAcct);
    return s;
}

// leahSecListSynced enumerates synchronizable rows. Caller frees out_svc/out_acct
// via leahSecFreeList.
static OSStatus leahSecListSynced(char ***out_svc, char ***out_acct, int *out_n) {
    *out_svc = NULL;
    *out_acct = NULL;
    *out_n = 0;
    const void *qKeys[] = { kSecClass, kSecAttrSynchronizable, kSecReturnAttributes, kSecMatchLimit };
    const void *qVals[] = { kSecClassGenericPassword, kCFBooleanTrue, kCFBooleanTrue, kSecMatchLimitAll };
    CFDictionaryRef query = CFDictionaryCreate(NULL, qKeys, qVals, 4,
                                               &kCFTypeDictionaryKeyCallBacks,
                                               &kCFTypeDictionaryValueCallBacks);
    CFTypeRef raw = NULL;
    OSStatus s = SecItemCopyMatching(query, &raw);
    CFRelease(query);
    if (s == errSecItemNotFound) return errSecSuccess; // empty list, not an error
    if (s != errSecSuccess) return s;
    if (CFGetTypeID(raw) != CFArrayGetTypeID()) { CFRelease(raw); return errSecSuccess; }
    CFArrayRef arr = (CFArrayRef)raw;
    CFIndex n = CFArrayGetCount(arr);
    char **svcs = (char **)calloc((size_t)n, sizeof(char *));
    char **accs = (char **)calloc((size_t)n, sizeof(char *));
    int kept = 0;
    for (CFIndex i = 0; i < n; i++) {
        CFDictionaryRef row = (CFDictionaryRef)CFArrayGetValueAtIndex(arr, i);
        CFStringRef svc = (CFStringRef)CFDictionaryGetValue(row, kSecAttrService);
        CFStringRef acct = (CFStringRef)CFDictionaryGetValue(row, kSecAttrAccount);
        if (svc == NULL || acct == NULL) continue;
        CFIndex svcLen = CFStringGetMaximumSizeForEncoding(CFStringGetLength(svc), kCFStringEncodingUTF8) + 1;
        CFIndex accLen = CFStringGetMaximumSizeForEncoding(CFStringGetLength(acct), kCFStringEncodingUTF8) + 1;
        char *svcBuf = (char *)malloc((size_t)svcLen);
        char *accBuf = (char *)malloc((size_t)accLen);
        if (!CFStringGetCString(svc, svcBuf, svcLen, kCFStringEncodingUTF8) ||
            !CFStringGetCString(acct, accBuf, accLen, kCFStringEncodingUTF8)) {
            free(svcBuf); free(accBuf); continue;
        }
        svcs[kept] = svcBuf;
        accs[kept] = accBuf;
        kept++;
    }
    *out_svc = svcs;
    *out_acct = accs;
    *out_n = kept;
    CFRelease(raw);
    return errSecSuccess;
}

static void leahSecFreeList(char **svc, char **acct, int n) {
    for (int i = 0; i < n; i++) { free(svc[i]); free(acct[i]); }
    free(svc); free(acct);
}
*/
import "C"

import (
	"context"
	"fmt"
	"sort"
	"unsafe"
)

// hasDarwinKeychain is the build-time toggle the test suite reads to decide
// whether the non-darwin error path is exercised.
const hasDarwinKeychain = true

// darwinKeystore is the SecItem-backed implementation. Each method is a thin
// CGo wrapper; OSStatus values surface as wrapped errors so callers can log.
type darwinKeystore struct{}

// New returns a darwin-backed ICloudKeystore.
func New() (ICloudKeystore, error) { return &darwinKeystore{}, nil }

func cString(s string) (*C.char, func()) {
	c := C.CString(s)
	return c, func() { C.free(unsafe.Pointer(c)) }
}

func (d *darwinKeystore) Put(_ context.Context, service, account string, data []byte, syncFlag bool) error {
	if service == "" || account == "" {
		return ErrBadKey
	}
	if len(data) == 0 {
		return ErrEmptyData
	}
	cs, freeS := cString(service)
	defer freeS()
	ca, freeA := cString(account)
	defer freeA()
	var flag C.int
	if syncFlag {
		flag = 1
	}
	s := C.leahSecPut(cs, ca, unsafe.Pointer(&data[0]), C.size_t(len(data)), flag)
	if s != 0 {
		return fmt.Errorf("icloud: SecItemAdd/Update failed: OSStatus %d", int32(s))
	}
	return nil
}

func (d *darwinKeystore) Get(_ context.Context, service, account string) ([]byte, error) {
	if service == "" || account == "" {
		return nil, ErrBadKey
	}
	cs, freeS := cString(service)
	defer freeS()
	ca, freeA := cString(account)
	defer freeA()
	var out unsafe.Pointer
	var outLen C.size_t
	s := C.leahSecGet(cs, ca, &out, &outLen)
	if s == C.errSecItemNotFound {
		return nil, ErrNotFound
	}
	if s != 0 {
		return nil, fmt.Errorf("icloud: SecItemCopyMatching failed: OSStatus %d", int32(s))
	}
	defer C.free(out)
	return C.GoBytes(out, C.int(outLen)), nil
}

func (d *darwinKeystore) Delete(_ context.Context, service, account string) error {
	if service == "" || account == "" {
		return ErrBadKey
	}
	cs, freeS := cString(service)
	defer freeS()
	ca, freeA := cString(account)
	defer freeA()
	s := C.leahSecDelete(cs, ca)
	if s == C.errSecItemNotFound || s == 0 {
		return nil // idempotent
	}
	return fmt.Errorf("icloud: SecItemDelete failed: OSStatus %d", int32(s))
}

func (d *darwinKeystore) ListSynced(_ context.Context) ([]string, error) {
	var svcPtr, accPtr **C.char
	var n C.int
	s := C.leahSecListSynced(&svcPtr, &accPtr, &n)
	if s != 0 {
		return nil, fmt.Errorf("icloud: SecItemCopyMatching(list) failed: OSStatus %d", int32(s))
	}
	if n == 0 {
		return []string{}, nil
	}
	defer C.leahSecFreeList(svcPtr, accPtr, n)
	count := int(n)
	svcSlice := unsafe.Slice(svcPtr, count)
	accSlice := unsafe.Slice(accPtr, count)
	out := make([]string, 0, count)
	for i := 0; i < count; i++ {
		out = append(out, C.GoString(svcSlice[i])+"/"+C.GoString(accSlice[i]))
	}
	sort.Strings(out)
	return out, nil
}
