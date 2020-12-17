
#ifndef V8_C_STACK_TRACE_H
#define V8_C_STACK_TRACE_H

#include <string>
#include <sstream>

inline v8::Local<v8::String> v8_StackTrace_FormatException(v8::Isolate* isolate, v8::Local<v8::Context> ctx, v8::TryCatch& try_catch) {
  v8::EscapableHandleScope handleScope(isolate);

  std::stringstream ss;
  ss << "Uncaught exception: ";

  std::string exceptionStr = v8_String_ToStdString(isolate, try_catch.Exception());
  ss << exceptionStr; // TODO(aroman) JSON-ify objects?

  if (!try_catch.Message().IsEmpty()) {
    if (!try_catch.Message()->GetScriptResourceName()->IsUndefined()) {
      ss << std::endl
         << "at " << v8_String_ToStdString(isolate, try_catch.Message()->GetScriptResourceName());

      v8::Maybe<int> line_no = try_catch.Message()->GetLineNumber(ctx);
      v8::Maybe<int> start = try_catch.Message()->GetStartColumn(ctx);
      v8::Maybe<int> end = try_catch.Message()->GetEndColumn(ctx);
      v8::MaybeLocal<v8::String> sourceLine = try_catch.Message()->GetSourceLine(ctx);

      if (line_no.IsJust()) {
        ss << ":" << line_no.ToChecked();
      }
      if (start.IsJust()) {
        ss << ":" << start.ToChecked();
      }
      if (!sourceLine.IsEmpty()) {
        ss << std::endl
           << "  " << v8_String_ToStdString(isolate, sourceLine.ToLocalChecked());
      }
      if (start.IsJust() && end.IsJust()) {
        ss << std::endl
           << "  ";
        for (int i = 0; i < start.ToChecked(); i++) {
          ss << " ";
        }
        for (int i = start.ToChecked(); i < end.ToChecked(); i++) {
          ss << "^";
        }
      }
    }
  }

  if (!try_catch.StackTrace(ctx).IsEmpty()) {
    ss << std::endl << "Stack trace: " << v8_String_ToStdString(isolate, try_catch.StackTrace(ctx).ToLocalChecked());
  }

  std::string string = ss.str();

  return handleScope.Escape(v8::String::NewFromUtf8(isolate, string.c_str(), v8::NewStringType::kNormal, string.length()).ToLocalChecked());
}

inline CallerInfo v8_StackTrace_CallerInfo(v8::Isolate* isolate) {
  std::string src_file, src_func;
  int line_number = 0, column = 0;

  v8::Local<v8::StackTrace> trace(v8::StackTrace::CurrentStackTrace(isolate, 1));

  if (trace->GetFrameCount() >= 1) {
    v8::Local<v8::StackFrame> frame(trace->GetFrame(isolate, 0));
    src_file = v8_String_ToStdString(isolate, frame->GetScriptName());
    src_func = v8_String_ToStdString(isolate, frame->GetFunctionName());
    line_number = frame->GetLineNumber();
    column = frame->GetColumn();
  }

  return CallerInfo{
    v8_String_Create(src_file),
    v8_String_Create(src_func),
    line_number,
    column
  };
}

#endif
