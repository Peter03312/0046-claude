package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"storyproof/geom"
	"storyproof/store"
)

// proofResult is the report body returned for a proof run.
type proofResult struct {
	ProjectID int64       `json:"projectId"`
	Version   int64       `json:"version"`
	Status    string      `json:"status"` // passed | failed
	Acts      []actResult `json:"acts"`
	// FailedAct is the 0-based index of the earliest failing act, or -1.
	FailedAct int `json:"failedAct"`
}

type actResult struct {
	ActID    int64          `json:"actId"`
	Name     string         `json:"name"`
	Position int            `json:"position"`
	Status   string         `json:"status"` // passed | failed | invalid
	BadCells []geom.BadCell `json:"badCells,omitempty"`
	Error    string         `json:"error,omitempty"`
}

// proofInput is the frozen snapshot embedded in each report: the complete
// project inputs at the moment the proof was created.
type proofInput struct {
	Project store.Project `json:"project"`
	Sheets  []store.Sheet `json:"sheets"`
	Acts    []store.Act   `json:"acts"`
}

func (s *Server) runProof(w http.ResponseWriter, r *http.Request) {
	pid, err := idParam(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	p, err := s.st.GetProject(pid)
	if err != nil {
		if mapStoreError(w, err) {
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	sheets, err := s.st.ListSheets(pid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	acts, err := s.st.ListActs(pid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	sheetByID := map[int64]store.Sheet{}
	for _, sh := range sheets {
		sheetByID[sh.ID] = sh
	}

	result := proofResult{ProjectID: pid, Version: p.Version, Status: "passed", FailedAct: -1, Acts: []actResult{}}
	failed := -1

	for ai, act := range acts {
		ar := actResult{ActID: act.ID, Name: act.Name, Position: act.Position, Status: "passed"}
		input, buildErr := buildActInput(act, sheetByID)
		if buildErr != nil {
			ar.Status = "invalid"
			ar.Error = buildErr.Error()
			result.Acts = append(result.Acts, ar)
			failed = ai
			result.Status = "failed"
			break
		}
		bad, evalErr := geom.Evaluate(input)
		if evalErr != nil {
			ar.Status = "invalid"
			ar.Error = evalErr.Error()
			result.Acts = append(result.Acts, ar)
			failed = ai
			result.Status = "failed"
			break
		}
		if len(bad) > 0 {
			ar.Status = "failed"
			ar.BadCells = bad
			result.Acts = append(result.Acts, ar)
			failed = ai
			result.Status = "failed"
			break
		}
		result.Acts = append(result.Acts, ar)
	}
	result.FailedAct = failed

	input := proofInput{Project: *p, Sheets: sheets, Acts: acts}
	resultJSON, _ := json.Marshal(result)
	inputJSON, _ := json.Marshal(input)
	rep, err := s.st.CreateReport(pid, result.Status, string(resultJSON), string(inputJSON))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Invalid geometry (e.g. a self-intersecting sheet) is a 422 so the UI
	// can surface the exact cause; a failed report is still recorded.
	if failed >= 0 && result.Acts[failed].Status == "invalid" {
		writeJSON(w, http.StatusUnprocessableEntity, reportPayload(rep))
		return
	}
	writeJSON(w, http.StatusCreated, reportPayload(rep))
}

// buildActInput applies each sheet's 0/90/180/270 rotation about the origin
// followed by the integer translation. Operations permute integer pairs, so
// transformed vertices remain integers. Sheets are ordered bottom to top by
// the act's sheet array order.
func buildActInput(act store.Act, sheetByID map[int64]store.Sheet) (geom.ActInput, error) {
	out := geom.ActInput{Name: act.Name}
	for i, as := range act.Sheets {
		sh, ok := sheetByID[as.SheetID]
		if !ok {
			return out, fmt.Errorf("act %q references missing sheet id %d", act.Name, as.SheetID)
		}
		vs := make([]geom.IntPoint, 0, len(sh.Vertices))
		for _, v := range sh.Vertices {
			q, ok := geom.Rotate(geom.IntPoint{X: v.X, Y: v.Y}, as.Rotation, as.TX, as.TY)
			if !ok {
				return out, fmt.Errorf("sheet %q: rotation must be 0/90/180/270", sh.Name)
			}
			vs = append(vs, q)
		}
		name := sh.Name
		out.Sheets = append(out.Sheets, geom.Sheet{
			ID:            fmt.Sprintf("sheet-%d", sh.ID),
			Name:          name,
			Vertices:      vs,
			R:             sh.R,
			G:             sh.G,
			B:             sh.B,
			OpacityMillis: sh.OpacityMillis,
		})
		_ = i
	}
	for _, rg := range act.Regions {
		vs := make([]geom.IntPoint, 0, len(rg.Vertices))
		for _, v := range rg.Vertices {
			vs = append(vs, geom.IntPoint{X: v.X, Y: v.Y})
		}
		out.Regions = append(out.Regions, geom.Region{
			ID:        fmt.Sprintf("region-%d", rg.ID),
			Name:      rg.Name,
			Kind:      rg.Kind,
			Vertices:  vs,
			R:         rg.R,
			G:         rg.G,
			B:         rg.B,
			Tolerance: rg.Tolerance,
		})
	}
	return out, nil
}
