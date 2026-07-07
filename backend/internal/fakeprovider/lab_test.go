package fakeprovider

import "testing"

func TestLabRegistersAndLooksUpProviders(t *testing.T) {
	lab := NewLab().
		Register(New("ollama", "local-ok")).
		Register(New("openai", "cloud-ok").AlwaysFail(nil))

	if names := lab.Names(); len(names) != 2 || names[0] != "ollama" || names[1] != "openai" {
		t.Fatalf("names = %v, want sorted [ollama openai]", names)
	}

	ollama, ok := lab.Get("ollama")
	if !ok {
		t.Fatalf("ollama not found")
	}
	if out, err := ollama.Generate("hi"); err != nil || out != "local-ok" {
		t.Fatalf("ollama should succeed: %q %v", out, err)
	}

	openai, _ := lab.Get("openai")
	if _, err := openai.Generate("hi"); err == nil {
		t.Fatalf("openai was configured to fail")
	}

	if _, ok := lab.Get("missing"); ok {
		t.Fatalf("unknown provider should not resolve")
	}
}
