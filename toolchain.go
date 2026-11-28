package sitec

// Toolchain ejecuta el toolchain de Go. Todo go list / go run / go build del
// compilador pasa por este puerto, para que el cacheo, el manejo de GOOS y la
// clasificación de errores existan en exactamente un lugar.
type Toolchain interface {
	List(dir string, args ...string) ([]byte, error)
	ListEnv(dir string, env []string, args ...string) ([]byte, error)
	Run(dir string, args ...string) ([]byte, error)
}

// WasmBuilder compila con TinyGo un Web Worker o el Worker del edge. La página
// nunca es WebAssembly: web/browser se compila a ES Module con goesm.
type WasmBuilder interface {
	Build(dir string) (WasmOutput, error)
}

// WasmOutput es el binario compilado y su nombre (con hash si se pidió). El
// pegamento que lo arranca lo pone quien lo sirve (js.WebWorker), siempre el de TinyGo.
type WasmOutput struct {
	Binary   []byte // el .wasm
	Filename string // su nombre, que el script del Worker referencia
}
