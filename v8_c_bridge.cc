
#include "v8_c_private.h"

#include <cstdlib>
#include <cstring>

extern "C"
{
  Version version = {V8_MAJOR_VERSION, V8_MINOR_VERSION, V8_BUILD_NUMBER, V8_PATCH_LEVEL};

  static std::unique_ptr<v8::Platform> _platform;

  void
  v8_Initialize()
  {

    v8::V8::InitializeICU();
    _platform = v8::platform::NewDefaultPlatform();
    v8::V8::InitializePlatform(_platform.get());
    const char *flags = "--harmony-rab-gsab";
    v8::V8::SetFlagsFromString(flags, strlen(flags));
    v8::V8::Initialize();

    StartupData data;
    auto isolate = v8_Isolate_New(data);
    printf("isolate: %p\n", isolate);

    // v8::V8::EnableWebAssemblyTrapHandler(true);

    return;
  }
}
