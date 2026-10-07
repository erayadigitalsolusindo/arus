import type id from './messages/id/index.ts';

// Bentuk pesan diturunkan dari kamus Indonesia (sumber kebenaran): bahasa lain wajib bertipe `Messages`.
export type Plural = { zero?: string; one?: string; other: string };

type Widen<T> = T extends string ? string : T extends Plural ? Plural : { [K in keyof T]: Widen<T[K]> };

export type Messages = Widen<typeof id>;

type Paths<T> = {
  [K in keyof T & string]: T[K] extends string | Plural ? K : `${K}.${Paths<T[K]>}`;
}[keyof T & string];

/** Kunci bertitik yang valid, mis. `auth.login.submit`. */
export type MessageKey = Paths<Messages>;

export type Params = Record<string, string | number>;
