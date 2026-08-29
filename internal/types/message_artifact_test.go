package types

import "testing"

func TestInferArtifactType(t *testing.T) {
	t.Parallel()
	tests := map[string]string{
		"deck.PPTX":   ArtifactTypePresentation,
		"report.html": ArtifactTypeWeb,
		"data.csv":    ArtifactTypeSpreadsheet,
		"book.xlsx":   ArtifactTypeSpreadsheet,
		"notes.pdf":   ArtifactTypeDocument,
		"cover.png":   ArtifactTypeImage,
		"archive.zip": ArtifactTypeOther,
	}
	for name, want := range tests {
		if got := InferArtifactType(name); got != want {
			t.Fatalf("%s: got %s want %s", name, got, want)
		}
	}
}
