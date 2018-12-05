
#include "v8_c_private.h"

#include "v8-inspector.h"

String StringFromStringView(v8::Isolate* isolate, const v8_inspector::StringView& view) {
  v8::MaybeLocal<v8::String> s;

  if (view.is8Bit()) {
    s = v8::String::NewFromOneByte(isolate, view.characters8(), v8::NewStringType::kNormal, view.length());
  } else {
    s = v8::String::NewFromTwoByte(isolate, view.characters16(), v8::NewStringType::kNormal, view.length());
  }

  return v8_String_Create(s.ToLocalChecked());
}

class Inspector : public v8_inspector::V8Inspector::Channel, public v8_inspector::V8InspectorClient {
public:
  Inspector(v8::Isolate* isolate, int inspectorId) : isolate_(isolate), inspectorId_(inspectorId) {
    inspector_ = v8_inspector::V8Inspector::create(isolate, this);
    session_ = inspector_->connect(1, this, v8_inspector::StringView());
  }
  void contextCreated(const v8_inspector::V8ContextInfo& contextInfo);
  void contextDestroyed(v8::Local<v8::Context> context);
  void dispatchProtocolMessage(v8_inspector::StringView& message);
  void sendResponse(int callId, std::unique_ptr<v8_inspector::StringBuffer> message) override;
  void sendNotification(std::unique_ptr<v8_inspector::StringBuffer> message) override;
  void flushProtocolNotifications() override;
  void runMessageLoopOnPause(int contextGroupId) override;
  void quitMessageLoopOnPause() override;

private:
  v8::Isolate* isolate_;
  std::unique_ptr<v8_inspector::V8Inspector> inspector_;
  std::unique_ptr<v8_inspector::V8InspectorSession> session_;
  int inspectorId_;
  bool runningNestedLoop_ = false;
  bool terminated_ = false;
};

void Inspector::contextCreated(const v8_inspector::V8ContextInfo& contextInfo) {
  inspector_->contextCreated(contextInfo);
}

void Inspector::contextDestroyed(v8::Local<v8::Context> context) {
  inspector_->contextDestroyed(context);
}

void Inspector::dispatchProtocolMessage(v8_inspector::StringView& message) {
  ISOLATE_SCOPE(isolate_);

  session_->dispatchProtocolMessage(message);
}

void Inspector::sendResponse(int callId, std::unique_ptr<v8_inspector::StringBuffer> message) {
  ISOLATE_SCOPE(isolate_);
  v8::HandleScope handle_scope(isolate);

  inspectorSendResponse(inspectorId_, callId, StringFromStringView(isolate, message->string()));
}

void Inspector::sendNotification(std::unique_ptr<v8_inspector::StringBuffer> message) {
  ISOLATE_SCOPE(isolate_);
  v8::HandleScope handle_scope(isolate);

  inspectorSendNotification(inspectorId_, StringFromStringView(isolate, message->string()));
}

void Inspector::flushProtocolNotifications() {
  ISOLATE_SCOPE(isolate_);
  inspectorFlushProtocolNotifications(inspectorId_);
}

void Inspector::runMessageLoopOnPause(int contextGroupId) {
	if (runningNestedLoop_) return;

	terminated_ = false;
	runningNestedLoop_ = true;

	while (!terminated_) {
    bool more = true;
		while (more) {
      ISOLATE_SCOPE(isolate_);
      more = v8::platform::PumpMessageLoop(platform, isolate);
    }
	}

	terminated_ = false;
	runningNestedLoop_ = false;
}

void Inspector::quitMessageLoopOnPause() {
	terminated_ = true;
}

extern "C" {
  InspectorPtr v8_Inspector_New(IsolatePtr pIsolate, int id) {
    ISOLATE_SCOPE(static_cast<v8::Isolate*>(pIsolate));
    Inspector *inspector = new Inspector(isolate, id);
    return (InspectorPtr)inspector;
  }

  void v8_Inspector_AddContext(InspectorPtr pInspector, ContextPtr pContext, const char* name) {
    VALUE_SCOPE(pContext);
    Inspector *inspector = static_cast<Inspector*>(pInspector);

    v8_inspector::StringView contextName((const uint8_t*)name, strlen(name));
    inspector->contextCreated(v8_inspector::V8ContextInfo(context, 1, contextName));
  }

  void v8_Inspector_RemoveContext(InspectorPtr pInspector, ContextPtr pContext) {
    VALUE_SCOPE(pContext);
    Inspector *inspector = static_cast<Inspector*>(pInspector);

    inspector->contextDestroyed(context);
  }

  void v8_Inspector_DispatchMessage(InspectorPtr pInspector, const char* message) {
    Inspector *inspector = static_cast<Inspector*>(pInspector);

    v8_inspector::StringView messageView((const uint8_t*)message, strlen(message));
    inspector->dispatchProtocolMessage(messageView);
  }
}
