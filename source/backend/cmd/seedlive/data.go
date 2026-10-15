package main

// Data master demo: toko ritel kebutuhan harian (sembako, minuman, makanan ringan, kebersihan). Harga dalam Rupiah,
// harga jual > HPP (margin ±15–35%) dan realistis untuk eceran Indonesia.

type itemDef struct {
	name     string
	cat      string
	brand    string
	unit     string
	cost     int // HPP awal per satuan dasar
	price    int // harga jual
	weight   int // bobot kepopuleran saat memilih barang kasir
	maxQty   int // qty maksimum per baris
	supplier int // indeks ke suppliers
	barcode  string
}

var units = []string{"Pcs", "Karung", "Galon", "Botol"}
var categories = []string{"Minuman", "Makanan", "Sembako", "Snack", "Kebersihan", "Perawatan"}

var suppliers = []struct{ code, name, contact, phone, address string }{
	{"SUP-001", "PT Sumber Makmur Sembako", "Hendra Wijaya", "081211110001", "Jl. Raya Pasar Induk No. 12, Magelang"},
	{"SUP-002", "CV Berkah Minuman", "Siti Rahmawati", "081211110002", "Jl. Pahlawan No. 45, Magelang"},
	{"SUP-003", "PT Indofood Distributor Jateng", "Agus Prasetyo", "081211110003", "Jl. Diponegoro No. 8, Semarang"},
	{"SUP-004", "UD Segar Jaya", "Bambang Sutrisno", "081211110004", "Jl. Tidar No. 21, Magelang"},
	{"SUP-005", "PT Unilever Distribusi Mitra", "Dewi Lestari", "081211110005", "Jl. Gatot Subroto No. 3, Yogyakarta"},
}

var brands = []string{"Indofood", "Mayora", "Unilever", "Sosro", "Danone", "Sari Roti", "Bimoli", "Kapal Api", "Wings", "Lokal"}

var memberNames = []string{
	"Budi Santoso", "Siti Aminah", "Ahmad Fauzi", "Rina Marlina", "Dedi Kurniawan", "Lestari Handayani",
	"Joko Susilo", "Wulan Sari", "Eko Prabowo", "Nur Hidayah", "Rudi Hartono", "Maya Anggraini",
}

// popularitas: barang kecil/harian lebih sering terbeli.
var itemDefs = []itemDef{
	{"Aqua 600 ml", "Minuman", "Lokal", "Botol", 2600, 4000, 10, 6, 1, "8999999000011"},
	{"Teh Botol Sosro 450 ml", "Minuman", "Sosro", "Botol", 3800, 5000, 8, 4, 1, "8999999000028"},
	{"Le Minerale 1,5 L", "Minuman", "Lokal", "Botol", 4000, 6000, 6, 4, 1, "8999999000035"},
	{"Coca-Cola 390 ml", "Minuman", "Lokal", "Botol", 4500, 6500, 5, 3, 1, "8999999000042"},
	{"Pocari Sweat 500 ml", "Minuman", "Lokal", "Botol", 6500, 9000, 4, 3, 1, "8999999000059"},
	{"Susu UHT Ultra 250 ml", "Minuman", "Danone", "Pcs", 5000, 7000, 5, 4, 1, "8999999000066"},
	{"Aqua Galon 19 L", "Minuman", "Lokal", "Galon", 16000, 22000, 4, 3, 1, "8999999000073"},
	{"Kopi Kapal Api Sachet", "Minuman", "Kapal Api", "Pcs", 1100, 1500, 9, 10, 2, "8999999000080"},
	{"Good Day Cappuccino Sachet", "Minuman", "Kapal Api", "Pcs", 1400, 2000, 7, 8, 2, "8999999000097"},
	{"Indomie Goreng", "Makanan", "Indofood", "Pcs", 2800, 3500, 10, 10, 2, "8999999000103"},
	{"Indomie Soto", "Makanan", "Indofood", "Pcs", 2700, 3400, 6, 8, 2, "8999999000110"},
	{"Mie Sedaap Goreng", "Makanan", "Wings", "Pcs", 2750, 3500, 5, 8, 2, "8999999000127"},
	{"Roti Tawar Sari Roti", "Makanan", "Sari Roti", "Pcs", 13000, 16500, 4, 2, 3, "8999999000134"},
	{"Telur Ayam 1 kg", "Sembako", "Lokal", "Pcs", 24000, 28000, 6, 3, 3, "8999999000141"},
	{"Chitato 68 g", "Snack", "Indofood", "Pcs", 8500, 11000, 5, 4, 2, "8999999000158"},
	{"Oreo 137 g", "Snack", "Mayora", "Pcs", 7000, 9500, 4, 3, 2, "8999999000165"},
	{"Beng-Beng", "Snack", "Mayora", "Pcs", 1900, 2500, 7, 8, 2, "8999999000172"},
	{"Better Wafer 100 g", "Snack", "Mayora", "Pcs", 6000, 8000, 3, 3, 2, "8999999000189"},
	{"Beras Premium 5 kg", "Sembako", "Lokal", "Karung", 62000, 72000, 4, 2, 0, "8999999000196"},
	{"Gula Pasir 1 kg", "Sembako", "Lokal", "Pcs", 14500, 17000, 5, 3, 0, "8999999000202"},
	{"Minyak Goreng Bimoli 2 L", "Sembako", "Bimoli", "Pcs", 32000, 37000, 5, 2, 0, "8999999000219"},
	{"Tepung Terigu Segitiga 1 kg", "Sembako", "Lokal", "Pcs", 11000, 13500, 3, 3, 0, "8999999000226"},
	{"Kecap Bango 275 ml", "Sembako", "Unilever", "Pcs", 14000, 17500, 3, 2, 0, "8999999000233"},
	{"Garam Dapur 250 g", "Sembako", "Lokal", "Pcs", 2500, 3500, 3, 4, 0, "8999999000240"},
	{"Rinso Deterjen 770 g", "Kebersihan", "Unilever", "Pcs", 17500, 22000, 3, 2, 4, "8999999000257"},
	{"Sunlight Pencuci Piring 755 ml", "Kebersihan", "Unilever", "Pcs", 13000, 16500, 3, 2, 4, "8999999000264"},
	{"Sabun Lifebuoy Batang", "Perawatan", "Unilever", "Pcs", 3000, 4500, 5, 4, 4, "8999999000271"},
	{"Shampoo Pantene Sachet", "Perawatan", "Unilever", "Pcs", 1500, 2200, 4, 8, 4, "8999999000288"},
	{"Pasta Gigi Pepsodent 190 g", "Perawatan", "Unilever", "Pcs", 12000, 15500, 3, 2, 4, "8999999000295"},
	{"Tisu Paseo 250 lembar", "Kebersihan", "Lokal", "Pcs", 11000, 14500, 3, 2, 4, "8999999000301"},
}
