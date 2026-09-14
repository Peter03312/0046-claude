package store

// JSON-facing data models. Vertices are encoded as JSON arrays.

type Vertex struct {
	X int64 `json:"x"`
	Y int64 `json:"y"`
}

type Project struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Version   int64  `json:"version"`
	Frozen    bool   `json:"frozen"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

type Sheet struct {
	ID            int64    `json:"id"`
	ProjectID     int64    `json:"projectId"`
	Name          string   `json:"name"`
	Vertices      []Vertex `json:"vertices"`
	R             int      `json:"r"`
	G             int      `json:"g"`
	B             int      `json:"b"`
	OpacityMillis int      `json:"opacityMillis"`
	Position      int      `json:"position"`
}

type Act struct {
	ID        int64      `json:"id"`
	ProjectID int64      `json:"projectId"`
	Name      string     `json:"name"`
	Position  int        `json:"position"`
	Sheets    []ActSheet `json:"sheets"`
	Regions   []Region   `json:"regions"`
}

type ActSheet struct {
	SheetID  int64 `json:"sheetId"`
	Stack    int   `json:"stack"`
	Rotation int64 `json:"rotation"`
	TX       int64 `json:"tx"`
	TY       int64 `json:"ty"`
}

type Region struct {
	ID        int64    `json:"id"`
	ActID     int64    `json:"actId"`
	Name      string   `json:"name"`
	Kind      string   `json:"kind"`
	Vertices  []Vertex `json:"vertices"`
	R         int      `json:"r"`
	G         int      `json:"g"`
	B         int      `json:"b"`
	Tolerance int      `json:"tolerance"`
	Position  int      `json:"position"`
}
