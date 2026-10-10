package main

// Printer menerima satu pekerjaan cetak berupa byte ESC/POS mentah. Implementasi per sistem operasi:
// Windows = antrean printer (winspool, tipe data RAW); lainnya = berkas perangkat (mis. /dev/usb/lp0).
type Printer interface {
	// Name = nama printer/perangkat yang dipakai (untuk /status dan log).
	Name() string
	// Print mengirim satu pekerjaan utuh; tidak boleh dipanggil bersamaan (server menyerialkan).
	Print(job string, data []byte) error
}
