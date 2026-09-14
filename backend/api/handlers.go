package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"storyproof/store"
)

type pointDTO struct {
	X int64 `json:"x"`
	Y int64 `json:"y"`
}

type projectReq struct {
	Name string `json:"name"`
}

type sheetReq struct {
	Name          string     `json:"name"`
	Vertices      []pointDTO `json:"vertices"`
	R             int        `json:"r"`
	G             int        `json:"g"`
	B             int        `json:"b"`
	OpacityMillis int        `json:"opacityMillis"`
}

type actSheetReq struct {
	SheetID  int64 `json:"sheetId"`
	Rotation int64 `json:"rotation"`
	TX       int64 `json:"tx"`
	TY       int64 `json:"ty"`
}

type regionReq struct {
	Name      string     `json:"name"`
	Kind      string     `json:"kind"`
	Vertices  []pointDTO `json:"vertices"`
	R         int        `json:"r"`
	G         int        `json:"g"`
	B         int        `json:"b"`
	Tolerance int        `json:"tolerance"`
}

type actReq struct {
	Name    string        `json:"name"`
	Sheets  []actSheetReq `json:"sheets"`
	Regions []regionReq   `json:"regions"`
}

func idParam(r *http.Request, name string) (int64, error) {
	v := chi.URLParam(r, name)
	id, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, errors.New("invalid id")
	}
	return id, nil
}

func toVertices(in []pointDTO) []store.Vertex {
	out := make([]store.Vertex, len(in))
	for i, p := range in {
		out[i] = store.Vertex{X: p.X, Y: p.Y}
	}
	return out
}

func (s *Server) createProject(w http.ResponseWriter, r *http.Request) {
	var req projectReq
	if err := decode(w, r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	p, err := s.st.CreateProject(req.Name)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (s *Server) listProjects(w http.ResponseWriter, r *http.Request) {
	ps, err := s.st.ListProjects()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if ps == nil {
		ps = []store.Project{}
	}
	writeJSON(w, http.StatusOK, ps)
}

func (s *Server) getProject(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	p, err := s.st.GetProject(id)
	if err != nil {
		if mapStoreError(w, err) {
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	sheets, _ := s.st.ListSheets(id)
	acts, _ := s.st.ListActs(id)
	writeJSON(w, http.StatusOK, map[string]any{
		"project": p, "sheets": nullSafeSheets(sheets), "acts": nullSafeActs(acts),
	})
}

func nullSafeSheets(ss []store.Sheet) []store.Sheet {
	if ss == nil {
		return []store.Sheet{}
	}
	return ss
}
func nullSafeActs(as []store.Act) []store.Act {
	if as == nil {
		return []store.Act{}
	}
	return as
}

func (s *Server) renameProject(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var req projectReq
	if err := decode(w, r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if err := s.st.RenameProject(id, req.Name); err != nil {
		if mapStoreError(w, err) {
			return
		}
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	p, _ := s.st.GetProject(id)
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) deleteProject(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.st.DeleteProject(id); err != nil {
		if mapStoreError(w, err) {
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) unfreezeProject(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.st.UnfreezeProject(id); err != nil {
		if mapStoreError(w, err) {
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	p, _ := s.st.GetProject(id)
	writeJSON(w, http.StatusOK, p)
}

func sheetInput(req *sheetReq) store.SheetInput {
	return store.SheetInput{
		Name: req.Name, Vertices: toVertices(req.Vertices),
		R: req.R, G: req.G, B: req.B, OpacityMillis: req.OpacityMillis,
	}
}

func (s *Server) createSheet(w http.ResponseWriter, r *http.Request) {
	pid, err := idParam(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var req sheetReq
	if err := decode(w, r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	sh, err := s.st.CreateSheet(pid, sheetInput(&req))
	if err != nil {
		if mapStoreError(w, err) {
			return
		}
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, sh)
}

func (s *Server) listSheets(w http.ResponseWriter, r *http.Request) {
	pid, err := idParam(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	ss, err := s.st.ListSheets(pid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, nullSafeSheets(ss))
}

func (s *Server) getSheet(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	sh, err := s.st.GetSheet(id)
	if err != nil {
		if mapStoreError(w, err) {
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, sh)
}

func (s *Server) updateSheet(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var req sheetReq
	if err := decode(w, r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	sh, err := s.st.UpdateSheet(id, sheetInput(&req))
	if err != nil {
		if mapStoreError(w, err) {
			return
		}
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, sh)
}

func (s *Server) deleteSheet(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.st.DeleteSheet(id); err != nil {
		if mapStoreError(w, err) {
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func actInput(req *actReq) (string, []store.ActSheetInput, []store.RegionInput) {
	sheets := make([]store.ActSheetInput, len(req.Sheets))
	for i, a := range req.Sheets {
		sheets[i] = store.ActSheetInput{SheetID: a.SheetID, Rotation: a.Rotation, TX: a.TX, TY: a.TY}
	}
	regions := make([]store.RegionInput, len(req.Regions))
	for i, g := range req.Regions {
		regions[i] = store.RegionInput{
			Name: g.Name, Kind: g.Kind, Vertices: toVertices(g.Vertices),
			R: g.R, G: g.G, B: g.B, Tolerance: g.Tolerance,
		}
	}
	return req.Name, sheets, regions
}

func (s *Server) createAct(w http.ResponseWriter, r *http.Request) {
	pid, err := idParam(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var req actReq
	if err := decode(w, r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	name, sheets, regions := actInput(&req)
	a, err := s.st.CreateAct(pid, name, sheets, regions)
	if err != nil {
		if mapStoreError(w, err) {
			return
		}
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, a)
}

func (s *Server) listActs(w http.ResponseWriter, r *http.Request) {
	pid, err := idParam(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	as, err := s.st.ListActs(pid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, nullSafeActs(as))
}

func (s *Server) getAct(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	a, err := s.st.GetAct(id)
	if err != nil {
		if mapStoreError(w, err) {
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, a)
}

func (s *Server) updateAct(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var req actReq
	if err := decode(w, r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	name, sheets, regions := actInput(&req)
	a, err := s.st.UpdateAct(id, name, sheets, regions)
	if err != nil {
		if mapStoreError(w, err) {
			return
		}
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, a)
}

func (s *Server) deleteAct(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.st.DeleteAct(id); err != nil {
		if mapStoreError(w, err) {
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listReports(w http.ResponseWriter, r *http.Request) {
	pid, err := idParam(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	rs, err := s.st.ListReports(pid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if rs == nil {
		rs = []store.Report{}
	}
	writeJSON(w, http.StatusOK, rs)
}

func (s *Server) getReport(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	rep, err := s.st.GetReport(id)
	if err != nil {
		if mapStoreError(w, err) {
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if rep.Expired {
		writeJSON(w, http.StatusGone, reportPayload(rep))
		return
	}
	writeJSON(w, http.StatusOK, reportPayload(rep))
}

func reportPayload(rep *store.Report) map[string]json.RawMessage {
	m := map[string]json.RawMessage{}
	b, _ := json.Marshal(rep)
	_ = json.Unmarshal(b, &m)
	m["result"] = json.RawMessage(rep.ResultJSON)
	m["input"] = json.RawMessage(rep.InputJSON)
	return m
}

func (s *Server) confirmReport(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	rep, err := s.st.ConfirmReport(id)
	if err != nil {
		if mapStoreError(w, err) {
			return
		}
		writeErr(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, reportPayload(rep))
}
