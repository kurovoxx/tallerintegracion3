package model

// GenerateRequest es el payload para generar contenido vía IA.
// En la tarea 4_5_6 este endpoint se conectará con Gemini para PDF→Markdown.
type GenerateRequest struct {
	// Texto de entrada (prompt, o contenido extraído de un PDF).
	Input string `json:"input"`
	// Tipo de operación solicitada (ej: "pdf-to-markdown", "chat", "summary").
	Operation string `json:"operation,omitempty"`
}

// GenerateResponse es la respuesta del microservicio IA.
type GenerateResponse struct {
	// Resultado generado (Markdown, texto, etc.).
	Output string `json:"output"`
	// Identificador de la operación (útil para trazabilidad).
	ID string `json:"id,omitempty"`
	// Modelo o proveedor usado (ej: "gemini-1.5-flash", "mock").
	Provider string `json:"provider,omitempty"`
}

// HealthResponse es la respuesta del endpoint /health.
type HealthResponse struct {
	Status  string `json:"status"`
	Service string `json:"service"`
}
