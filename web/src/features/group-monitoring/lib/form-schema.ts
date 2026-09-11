/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/

import { z } from 'zod'

// Mirrors setting/operation_setting/group_monitoring_setting.go so the form
// rejects what the API would reject anyway.
export const MAX_MONITORED_GROUPS = 50
export const MAX_MONITORED_MODELS_PER_GROUP = 50
export const MAX_GROUP_DESCRIPTION_LENGTH = 500

export const groupMonitoringFormSchema = z.object({
  enabled: z.boolean(),
  bucket_minutes: z.union([
    z.literal(5),
    z.literal(15),
    z.literal(30),
    z.literal(60),
  ]),
  groups: z
    .array(
      z.object({
        group: z.string().trim().min(1).max(64),
        description: z.string().trim().max(MAX_GROUP_DESCRIPTION_LENGTH),
        // null means "no restriction"; an empty array means "administrators
        // only". Keep both reachable, they are different settings.
        visible_to_groups: z
          .array(z.string().trim().min(1).max(64))
          .max(MAX_MONITORED_GROUPS)
          .nullable(),
        models: z
          .array(z.string().trim().min(1).max(255))
          .min(1)
          .max(MAX_MONITORED_MODELS_PER_GROUP),
      })
    )
    .max(MAX_MONITORED_GROUPS),
})

export type GroupMonitoringFormValues = z.infer<
  typeof groupMonitoringFormSchema
>
