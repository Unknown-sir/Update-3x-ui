import { z } from 'zod';

export const OpenvpnClientSchema = z.object({
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
export type OpenvpnClient = z.infer<typeof OpenvpnClientSchema>;

export const OpenvpnInboundSettingsSchema = z.object({
  proto: z.enum(['udp', 'tcp']).default('udp'),
  subnet: z.string().default('10.8.0.0'),
  netmask: z.string().default('255.255.255.0'),
  cipher: z.string().default('AES-256-GCM'),
  auth: z.string().default('SHA256'),
  speedLimitMbps: z.number().int().min(0).default(0),
  clients: z.array(OpenvpnClientSchema).default([]),
});
export type OpenvpnInboundSettings = z.infer<typeof OpenvpnInboundSettingsSchema>;
