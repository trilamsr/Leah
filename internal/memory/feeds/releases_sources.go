package feeds

// DefaultReleaseSources returns the curated GitHub-repo list for a SWE+AI/ML
// operator's release-notes watch. Each URL was verified to return HTTP 200 at
// authorship time; renames (e.g. ggerganov/llama.cpp -> ggml-org/llama.cpp)
// are recorded at the resolved owner so the feed doesn't depend on a 301.
//
// Composition: Anthropic SDKs + claude-code, OpenAI SDKs, MCP reference SDKs
// + servers, Go runtime, PyTorch/transformers, llama.cpp/vllm/ollama. Kept
// to 16 entries — wide enough to surface a library-shift, narrow enough that
// a daily fetch stays under a few seconds.
func DefaultReleaseSources() []ReleaseSource {
	return []ReleaseSource{
		// Anthropic
		{Name: "anthropics/claude-code", URL: "https://github.com/anthropics/claude-code/releases.atom"},
		{Name: "anthropics/anthropic-sdk-python", URL: "https://github.com/anthropics/anthropic-sdk-python/releases.atom"},
		{Name: "anthropics/anthropic-sdk-typescript", URL: "https://github.com/anthropics/anthropic-sdk-typescript/releases.atom"},
		// OpenAI
		{Name: "openai/openai-python", URL: "https://github.com/openai/openai-python/releases.atom"},
		{Name: "openai/openai-node", URL: "https://github.com/openai/openai-node/releases.atom"},
		{Name: "openai/openai-go", URL: "https://github.com/openai/openai-go/releases.atom"},
		// MCP
		{Name: "modelcontextprotocol/python-sdk", URL: "https://github.com/modelcontextprotocol/python-sdk/releases.atom"},
		{Name: "modelcontextprotocol/typescript-sdk", URL: "https://github.com/modelcontextprotocol/typescript-sdk/releases.atom"},
		{Name: "modelcontextprotocol/servers", URL: "https://github.com/modelcontextprotocol/servers/releases.atom"},
		// Go runtime
		{Name: "golang/go", URL: "https://github.com/golang/go/releases.atom"},
		// ML libraries
		{Name: "pytorch/pytorch", URL: "https://github.com/pytorch/pytorch/releases.atom"},
		{Name: "huggingface/transformers", URL: "https://github.com/huggingface/transformers/releases.atom"},
		{Name: "langchain-ai/langchain", URL: "https://github.com/langchain-ai/langchain/releases.atom"},
		// Local-inference runtimes
		{Name: "ggml-org/llama.cpp", URL: "https://github.com/ggml-org/llama.cpp/releases.atom"},
		{Name: "vllm-project/vllm", URL: "https://github.com/vllm-project/vllm/releases.atom"},
		{Name: "ollama/ollama", URL: "https://github.com/ollama/ollama/releases.atom"},
	}
}
