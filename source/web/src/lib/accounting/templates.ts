// Template jurnal cepat: pasangan akun bawaan (bagan akun retail) + jenis. Hanya mengisi akun & keterangan;
// nominal diketik pengguna. Template ditampilkan hanya bila semua kodenya ada di bagan akun tenant.

export type TemplateId = 'capital' | 'toBank' | 'fromBank' | 'rent' | 'utilities' | 'salary' | 'transport' | 'bankFee' | 'payable' | 'receivable' | 'otherIncome' | 'equipment';
export type JournalTemplate = { id: TemplateId; type: 'JU' | 'KM' | 'KK' | 'TK'; debit: string; credit: string };

export const journalTemplates: JournalTemplate[] = [
  { id: 'capital', type: 'KM', debit: '1110', credit: '3100' },
  { id: 'toBank', type: 'TK', debit: '1120', credit: '1110' },
  { id: 'fromBank', type: 'TK', debit: '1110', credit: '1120' },
  { id: 'rent', type: 'KK', debit: '6110', credit: '1110' },
  { id: 'utilities', type: 'KK', debit: '6120', credit: '1110' },
  { id: 'salary', type: 'KK', debit: '6100', credit: '1110' },
  { id: 'transport', type: 'KK', debit: '6140', credit: '1110' },
  { id: 'bankFee', type: 'KK', debit: '6160', credit: '1120' },
  { id: 'payable', type: 'KK', debit: '2110', credit: '1110' },
  { id: 'receivable', type: 'KM', debit: '1110', credit: '1210' },
  { id: 'otherIncome', type: 'KM', debit: '1110', credit: '4900' },
  { id: 'equipment', type: 'KK', debit: '1410', credit: '1110' }
];
