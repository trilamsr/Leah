#import <Foundation/Foundation.h>

static NSNetService* g_service = nil;

int leah_bonjour_publish(const char* name, int port) {
    NSString* nsName = [NSString stringWithUTF8String:name];
    NSNetService* svc = [[NSNetService alloc]
        initWithDomain:@""
                  type:@"_leah-sync._tcp."
                  name:nsName
                  port:(int)port];
    if (svc == nil) {
        return 1;
    }
    NSDictionary* txt = @{
        @"v":   [@"1"     dataUsingEncoding:NSUTF8StringEncoding],
        @"node":[nsName   dataUsingEncoding:NSUTF8StringEncoding],
        @"cap": [@"1"     dataUsingEncoding:NSUTF8StringEncoding],
    };
    NSData* txtData = [NSNetService dataFromTXTRecordDictionary:txt];
    if (txtData != nil) {
        [svc setTXTRecordData:txtData];
    }
    [svc publish];
    g_service = svc;
    return 0;
}

void leah_bonjour_stop(void) {
    if (g_service != nil) {
        [g_service stop];
        g_service = nil;
    }
}
