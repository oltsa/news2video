// Filename: components/render/AssetSelector.tsx
"use client";

import { useState, useEffect, useCallback } from 'react';
import * as api from '@/lib/api';
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { File, Folder, ChevronRight, Home } from 'lucide-react';

interface AssetSelectorProps {
  projectId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onAssetSelect: (assetPath: string) => void;
}

export function AssetSelector({ projectId, open, onOpenChange, onAssetSelect }: AssetSelectorProps) {
  const [assets, setAssets] = useState<api.AssetItem[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [currentPath, setCurrentPath] = useState<string[]>([]);

  const fetchAssets = useCallback(async () => {
    setIsLoading(true);
    const pathString = currentPath.join('/');
    try {
      const response = await api.listAssets(projectId, pathString);
      const data = response.data || [];
      setAssets(data.sort((a, b) => {
        if (a.is_dir !== b.is_dir) return a.is_dir ? -1 : 1;
        return a.key.localeCompare(b.key);
      }));
    } catch (error) {
      console.error("Failed to load assets for selector", error);
    } finally {
      setIsLoading(false);
    }
  }, [projectId, currentPath]);

  useEffect(() => {
    if (open) {
      fetchAssets();
    }
  }, [open, fetchAssets]);

  const handleFolderClick = (folderName: string) => {
    setCurrentPath(prev => [...prev, folderName]);
  };

  const handleBreadcrumbClick = (index: number) => {
    setCurrentPath(prev => prev.slice(0, index + 1));
  };

  const handleFileSelect = (fileName: string) => {
    const fullPath = [...currentPath, fileName].join('/');
    const placeholder = `{{PROJECT_ASSETS}}/${fullPath}`;
    onAssetSelect(placeholder);
    onOpenChange(false);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[600px]">
        <DialogHeader>
          <DialogTitle>Select an Asset</DialogTitle>
          <div className="flex items-center gap-1 text-sm text-muted-foreground pt-2">
            <Home className="h-4 w-4" />
            <Button variant="link" className="p-0 h-auto" onClick={() => setCurrentPath([])}>Root</Button>
            {currentPath.map((segment, index) => (
              <div key={index} className="flex items-center gap-1">
                <ChevronRight className="h-4 w-4" />
                <Button variant="link" className="p-0 h-auto" onClick={() => handleBreadcrumbClick(index)}>{segment}</Button>
              </div>
            ))}
          </div>
        </DialogHeader>
        <div className="max-h-[50vh] overflow-y-auto border rounded-md p-2">
          {isLoading ? (
            <div className="space-y-2">
              <Skeleton className="h-10 w-full" />
              <Skeleton className="h-10 w-full" />
              <Skeleton className="h-10 w-full" />
            </div>
          ) : (
            <ul className="space-y-1">
              {assets.map(asset => (
                <li key={asset.key}>
                  <button
                    onClick={() => asset.is_dir ? handleFolderClick(asset.key) : handleFileSelect(asset.key)}
                    className="w-full flex items-center gap-3 p-2 text-sm rounded-md hover:bg-muted text-left"
                  >
                    {asset.is_dir ? <Folder className="h-5 w-5 shrink-0" /> : <File className="h-5 w-5 shrink-0" />}
                    <span>{asset.key}</span>
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}