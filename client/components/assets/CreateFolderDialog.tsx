// Filename: components/assets/CreateFolderDialog.tsx
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

interface CreateFolderDialogProps {
  projectId: string;
  currentPath: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCreateSuccess: () => void;
}

export function CreateFolderDialog({ projectId, currentPath, open, onOpenChange, onCreateSuccess }: CreateFolderDialogProps) {
  const [folderName, setFolderName] = useState('');
  const [isCreating, setIsCreating] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleSubmit = async () => {
    if (!folderName) {
      setError('Folder name cannot be empty.');
      return;
    }

    setIsCreating(true);
    setError(null);

    try {
      await api.createFolder(projectId, {
        path: currentPath,
        folderName: folderName,
      });
      onCreateSuccess();
      onOpenChange(false);
    } catch (err) {
      console.error('Failed to create folder', err);
      setError('Failed to create folder. Please try again.');
    } finally {
      setIsCreating(false);
    }
  };

  const handleOpenChange = (isOpen: boolean) => {
    if (!isOpen) {
      setFolderName('');
      setError(null);
    }
    onOpenChange(isOpen);
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-[425px]">
        <DialogHeader>
          <DialogTitle>Create New Folder</DialogTitle>
          <DialogDescription>
            Enter a name for your new folder in `{currentPath || 'Assets Root'}`.
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-4 py-4">
          <div className="grid grid-cols-4 items-center gap-4">
            <Label htmlFor="name" className="text-right">Name</Label>
            <Input
              id="name"
              value={folderName}
              onChange={(e) => setFolderName(e.target.value)}
              className="col-span-3"
              placeholder="e.g., Summer Campaign"
            />
          </div>
          {error && <p className="col-span-4 text-center text-sm text-red-500">{error}</p>}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={isCreating}>Cancel</Button>
          <Button onClick={handleSubmit} disabled={isCreating || !folderName}>
            {isCreating ? 'Creating...' : 'Create Folder'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}