#import <Vision/Vision.h>
#import <CoreGraphics/CoreGraphics.h>

typedef struct { const char* text; int x; int y; int w; int h; double conf; } OCRHit;

int leah_ocr_recognize(const unsigned char* px, int w, int h, int bpp, OCRHit** out_hits) {
    CGColorSpaceRef cs = (bpp == 1) ? CGColorSpaceCreateDeviceGray() : CGColorSpaceCreateDeviceRGB();
    CGContextRef ctx = CGBitmapContextCreate((void*)px, w, h, 8, w*bpp, cs, (bpp == 1) ? kCGImageAlphaNone : kCGImageAlphaPremultipliedLast);
    CGImageRef cgImg = CGBitmapContextCreateImage(ctx);
    VNRecognizeTextRequest* req = [[VNRecognizeTextRequest alloc] init];
    req.recognitionLevel = VNRequestTextRecognitionLevelAccurate;
    req.usesLanguageCorrection = YES;
    VNImageRequestHandler* handler = [[VNImageRequestHandler alloc] initWithCGImage:cgImg options:@{}];
    NSError* err = nil;
    [handler performRequests:@[req] error:&err];
    NSArray<VNRecognizedTextObservation*>* results = req.results;
    int n = (int)results.count;
    if (n == 0) { *out_hits = NULL; CGImageRelease(cgImg); CGContextRelease(ctx); CGColorSpaceRelease(cs); return 0; }
    OCRHit* hits = calloc(n, sizeof(OCRHit));
    for (int i = 0; i < n; i++) {
        VNRecognizedTextObservation* o = results[i];
        VNRecognizedText* top = [[o topCandidates:1] firstObject];
        hits[i].text = strdup([top.string UTF8String]);
        hits[i].x = (int)(o.boundingBox.origin.x * w);
        hits[i].y = (int)((1.0 - o.boundingBox.origin.y - o.boundingBox.size.height) * h);
        hits[i].w = (int)(o.boundingBox.size.width * w);
        hits[i].h = (int)(o.boundingBox.size.height * h);
        hits[i].conf = top.confidence;
    }
    *out_hits = hits;
    CGImageRelease(cgImg); CGContextRelease(ctx); CGColorSpaceRelease(cs);
    return n;
}

void leah_ocr_free(OCRHit* hits, int n) {
    for (int i = 0; i < n; i++) free((void*)hits[i].text);
    free(hits);
}
