
#include "v8_c_private.h"

auto allocator = v8::ArrayBuffer::Allocator::NewDefaultAllocator();
void v8_Isolate_AddImportModuleDynamicallyCallbackHandler(IsolatePtr pIsolate);
void BeforeCallEnteredCallback(v8::Isolate *isolate);
void CallCompletedCallback(v8::Isolate *isolate);


extern "C"
{
  // StartupData v8_CreateSnapshotDataBlob(const char *js)
  // {
  //   v8::StartupData data = v8::V8::CreateSnapshotDataBlob(js);
  //   return StartupData{data.data, data.raw_size};
  // }

  IsolatePtr v8_Isolate_New(void *data, StartupData startupData)
  {
    std::shared_ptr<v8::ArrayBuffer::Allocator> allocator(v8::ArrayBuffer::Allocator::NewDefaultAllocator());

    v8::Isolate::CreateParams createParams;
    createParams.array_buffer_allocator_shared = allocator;

    if (startupData.length > 0 && startupData.data != NULL)
    {
      v8::StartupData *data = new v8::StartupData;
      data->data = startupData.data;
      data->raw_size = startupData.length;
      createParams.snapshot_blob = data;
    }

    auto isolate = v8::Isolate::New(createParams);
    isolate->SetData(0, data);

    // isolate->Enter();
    v8_Isolate_AddImportModuleDynamicallyCallbackHandler(isolate);
    isolate->AddBeforeCallEnteredCallback(BeforeCallEnteredCallback);
    isolate->AddCallCompletedCallback(CallCompletedCallback);

    return isolate;
  }

  void v8_Isolate_Enter(IsolatePtr pIsolate)
  {
    v8::Isolate *isolate = static_cast<v8::Isolate *>(pIsolate);
    v8::Locker __locker(isolate);
    isolate->Enter();
  }

  void v8_Isolate_Exit(IsolatePtr pIsolate)
  {
    v8::Isolate *isolate = static_cast<v8::Isolate *>(pIsolate);
    v8::Locker __locker(isolate);
    isolate->Exit();
  }

  Error v8_Isolate_EnqueueMicrotask(IsolatePtr pIsolate, ContextPtr pContext, ValuePtr pFunction)
  {
    VALUE_SCOPE(pContext);
    v8::MicrotasksScope microtasksScope(isolate, v8::MicrotasksScope::kRunMicrotasks);

    v8::Local<v8::Value> value = static_cast<Value *>(pFunction)->Get(isolate);
    if (!value->IsFunction())
    {
      return v8_String_Create("Not a function");
    }
    v8::Local<v8::Function> function = v8::Local<v8::Function>::Cast(value);

    isolate->EnqueueMicrotask(function);

    // return Error{NULL, 0};
    return Error{NULL, 0};
  }

  void v8_Isolate_PerformMicrotaskCheckpoint(IsolatePtr pIsolate)
  {
    ISOLATE_SCOPE(static_cast<v8::Isolate *>(pIsolate));

    isolate->PerformMicrotaskCheckpoint();
  }

  void v8_Isolate_Terminate(IsolatePtr isolate_ptr)
  {
    v8::Isolate *isolate = static_cast<v8::Isolate *>(isolate_ptr);
    isolate->TerminateExecution();
  }

  void v8_Isolate_RequestGarbageCollectionForTesting(IsolatePtr pIsolate)
  {
    ISOLATE_SCOPE(static_cast<v8::Isolate *>(pIsolate));

    isolate->RequestGarbageCollectionForTesting(v8::Isolate::kFullGarbageCollection);
  }

  HeapStatistics v8_Isolate_GetHeapStatistics(IsolatePtr pIsolate)
  {
    if (pIsolate == NULL)
    {
      return HeapStatistics{0};
    }

    ISOLATE_SCOPE(static_cast<v8::Isolate *>(pIsolate));

    v8::HeapStatistics hs;
    isolate->GetHeapStatistics(&hs);

    return HeapStatistics{
        hs.total_heap_size(),
        hs.total_heap_size_executable(),
        hs.total_physical_size(),
        hs.total_available_size(),
        hs.used_heap_size(),
        hs.heap_size_limit(),
        hs.malloced_memory(),
        hs.peak_malloced_memory(),
        hs.does_zap_garbage()};
  }

  void v8_Isolate_LowMemoryNotification(IsolatePtr pIsolate)
  {
    if (pIsolate == NULL)
    {
      return;
    }
    ISOLATE_SCOPE(static_cast<v8::Isolate *>(pIsolate));
    isolate->LowMemoryNotification();
  }

  void v8_Isolate_Release(IsolatePtr isolate_ptr)
  {
    if (isolate_ptr == nullptr)
    {
      return;
    }
    v8::Isolate *isolate = static_cast<v8::Isolate *>(isolate_ptr);
    isolate->Dispose();
  }
}

void BeforeCallEnteredCallback(v8::Isolate *isolate) {
  beforeCallEnteredCallback(isolate->GetData(0));
}

void CallCompletedCallback(v8::Isolate *isolate) {
  callCompletedCallback(isolate->GetData(0));
}
