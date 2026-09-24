// Devices: the default one, and the CPU, which every machine has.
import "gpu"

let d = gpu.Default()
print(d.Name == "metal" || d.Name == "cpu", gpu.CPU().Name, gpu.CPU().IsCPU)
print(gpu.Devices().count >= 1, gpu.Devices()[gpu.Devices().count - 1].IsCPU)
// want: true cpu true
// want: true true
