import { z } from 'zod';

export const CiscoClientSchema = z.object({
  email: z.string().min(1),
  password: z.string().default(''),
  limitIp: z.number().int().min(0).default(0),
  totalGB: z.number().int().min(0).default(0),
  expiryTime: z.number().int().default(0),
  speedLimitMbps: z.number().int().min(0).default(0),
  enable: z.boolean().default(true),
  subId: z.string().default(''),
  comment: z.string().default(''),
  reset: z.number().int().min(0).default(0),
});
export type CiscoClient = z.infer<typeof CiscoClientSchema>;

export const CiscoInboundSettingsSchema = z.object({
  auth: z.string().default('plain'),
  subnet: z.string().default('10.9.0.0/24'),
  dns: z.array(z.string()).default(['8.8.8.8']),
  speedLimitMbps: z.number().int().min(0).default(0),
  clients: z.array(CiscoClientSchema).default([]),
});
export type CiscoInboundSettings = z.infer<typeof CiscoInboundSettingsSchema>;
