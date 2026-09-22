// Filename: components/connections/AddConnectionDialog.tsx
"use client";

import { useState } from 'react';
import * as api from '@/lib/api';
import { Button } from '@/components/ui/button';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';

interface AddConnectionDialogProps {
  projectId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSuccess: () => void;
}

type Platform = 's3' | 'webhook' | 'tiktok' | 'youtube';

export function AddConnectionDialog({ projectId, open, onOpenChange, onSuccess }: AddConnectionDialogProps) {
  const API_BASE = process.env.NEXT_PUBLIC_API_BASE_URL ?? '';
  const [platform, setPlatform] = useState<Platform | ''>('');
  const [webhookConfig, setWebhookConfig] = useState<api.WebhookConfig>({ url: '' });
  const [isCreating, setIsCreating] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handlePlatformChange = (value: Platform) => {
    setPlatform(value);
  };

  const handleSubmit = async () => {
    if (!platform) return;

    let requestData: api.CreateConnectionRequest;

    if (platform === 'tiktok') {
      window.location.href = `${API_BASE}/oauth/tiktok/start?project_id=${projectId}`;
      return;
    }

    if (platform === 'youtube') {
      window.location.href = `${API_BASE}/oauth/youtube/start?project_id=${projectId}`;
      return;
    }

    if (platform === 's3') {
      // S3 config is an empty object, as defined in our types.
      requestData = { platform: 's3', config: {} };
      return
    } else if (platform === 'webhook') {
      if (!webhookConfig.url) {
        setError("Webhook URL is required.");
        return;
      }
      requestData = { platform: 'webhook', config: webhookConfig };
    } else {
      // This case should not be reachable with the current UI
      console.error("Invalid platform selected");
      return;
    }

    setIsCreating(true);
    setError(null);
    try {
      await api.createConnection(projectId, requestData);
      onSuccess();
      onOpenChange(false);
    } catch (err) {
      console.error('Failed to create connection', err);
      setError('Failed to create connection. Please check your inputs.');
    } finally {
      setIsCreating(false);
    }
  };

  // Reset state when the dialog is closed
  const handleOpenChange = (isOpen: boolean) => {
    if (!isOpen) {
      setPlatform('');
      setWebhookConfig({ url: '' });
      setError(null);
    }
    onOpenChange(isOpen);
  };

  const isFormValid = () => {
    if (!platform) return false;
    if (platform === 'webhook' && (!webhookConfig.url || webhookConfig.url.trim() === '')) return false;
    // S3 is always valid once selected
    return true;
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-[425px]">
        <DialogHeader>
          <DialogTitle>Add New Connection</DialogTitle>
          <DialogDescription>Configure a new destination for your videos.</DialogDescription>
        </DialogHeader>
        <div className="grid gap-4 py-4">
          <div className="grid grid-cols-4 items-center gap-4">
            <Label htmlFor="platform" className="text-right">Platform</Label>
            <Select value={platform} onValueChange={(v) => handlePlatformChange(v as Platform)}>
              <SelectTrigger className="col-span-3">
                <SelectValue placeholder="Select a platform" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="s3">S3</SelectItem>
                <SelectItem value="webhook">Webhook</SelectItem>
                <SelectItem value="tiktok">TikTok</SelectItem>
                <SelectItem value="youtube">YouTube</SelectItem>
              </SelectContent>
            </Select>
          </div>

          {/* Dynamic Form Fields */}
          {platform === 'webhook' && (
            <div className="grid grid-cols-4 items-center gap-4">
              <Label htmlFor="webhook-url" className="text-right">URL</Label>
              <Input
                id="webhook-url"
                placeholder="https://your-endpoint.com/hook"
                value={webhookConfig.url}
                onChange={(e) => setWebhookConfig(prev => ({ ...prev, url: e.target.value }))}
                className="col-span-3"
              />
            </div>
          )}
          {platform === 's3' && (
             <p className="col-span-4 text-center text-sm text-muted-foreground p-4 bg-muted rounded-md">
                S3 connection uses the systems default bucket. No extra configuration is needed.
            </p>
          )}

          {error && <p className="col-span-4 text-center text-sm text-red-500">{error}</p>}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={isCreating}>Cancel</Button>
          <Button onClick={handleSubmit} disabled={isCreating || !isFormValid()}>
            {isCreating ? 'Creating...' : 'Create Connection'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}