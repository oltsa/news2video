// Filename: components/assets/UploadAssetDialog.tsx
"use client";

import { useState } from 'react';
import * as api from '@/lib/api';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import axios from 'axios';

interface UploadAssetDialogProps {
  projectId: string;
  currentPath: string; // The dialog still needs to know the current path
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onUploadSuccess: () => void;
}

export function UploadAssetDialog({ projectId, currentPath, open, onOpenChange, onUploadSuccess }: UploadAssetDialogProps) {
  const [file, setFile] = useState<File | null>(null);
  const [isUploading, setIsUploading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    if (e.target.files && e.target.files.length > 0) {
      setFile(e.target.files[0]);
    }
  };

  const handleSubmit = async () => {
    if (!file) {
      setError('Please select a file.');
      return;
    }

    setIsUploading(true);
    setError(null);

    try {
      // 1. Get the pre-signed URL from our backend
      const urlResponse = await api.getUploadUrl(projectId, {
        filename: file.name,
        contentType: file.type,
        // The `assetType` is the full path we are currently in
        assetType: currentPath,
      });

      // 2. Upload the file directly to S3 using the provided URL
      await axios.put(urlResponse.data.url, file, {
        headers: {
          'Content-Type': file.type,
        },
      });

      // 3. Success!
      onUploadSuccess();
      onOpenChange(false); // Close the dialog
    } catch (err) {
      console.error('Upload failed', err);
      setError('File upload failed. Please try again.');
    } finally {
      setIsUploading(false);
    }
  };

  // Reset state when dialog is closed
  const handleOpenChange = (isOpen: boolean) => {
    if (!isOpen) {
      setFile(null);
      setError(null);
    }
    onOpenChange(isOpen);
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Upload to `{currentPath || 'Assets Root'}`</DialogTitle>
          <DialogDescription>
            Select a file to upload to the current directory.
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-4 py-4">
          <div className="grid grid-cols-4 items-center gap-4">
            <Label htmlFor="file" className="text-right">File</Label>
            <Input id="file" type="file" onChange={handleFileChange} className="col-span-3" />
          </div>
          {error && <p className="col-span-4 text-center text-sm text-red-500">{error}</p>}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={isUploading}>Cancel</Button>
          <Button onClick={handleSubmit} disabled={isUploading || !file}>
            {isUploading ? 'Uploading...' : 'Upload'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}