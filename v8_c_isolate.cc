
#include "v8_c_private.h"

auto allocator = v8::ArrayBuffer::Allocator::NewDefaultAllocator();

extern "C" {
  StartupData v8_CreateSnapshotDataBlob(const char* js) {
    v8::StartupData data = v8::V8::CreateSnapshotDataBlob(js);
    return StartupData{data.data, data.raw_size};
  }

  IsolatePtr v8_Isolate_New(StartupData startupData) {
    v8::Isolate::CreateParams createParams;

    createParams.array_buffer_allocator = allocator;

    if (startupData.length > 0 && startupData.data != NULL) {
      v8::StartupData* data = new v8::StartupData;
      data->data = startupData.data;
      data->raw_size = startupData.length;
      createParams.snapshot_blob = data;
    }

    return static_cast<IsolatePtr>(v8::Isolate::New(createParams));
  }

  void v8_Isolate_Terminate(IsolatePtr isolate_ptr) {
    v8::Isolate* isolate = static_cast<v8::Isolate*>(isolate_ptr);
    isolate->TerminateExecution();
  }

  void v8_Isolate_Release(IsolatePtr isolate_ptr) {
    if (isolate_ptr == nullptr) {
      return;
    }
    v8::Isolate* isolate = static_cast<v8::Isolate*>(isolate_ptr);
    isolate->Dispose();
  }
}
