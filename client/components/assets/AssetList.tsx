// Filename: components/assets/AssetList.tsx
"use client";

import { useState, useEffect, useCallback } from 'react';
import * as api from '@/lib/api';
import { AssetItem } from '@/lib/api';
import { format } from 'date-fns';
import { File, Folder, MoreHorizontal, Trash2, Upload, ChevronRight, Home, FolderPlus } from 'lucide-react';
import { UploadAssetDialog } from './UploadAssetDialog';
import { CreateFolderDialog } from './CreateFolderDialog';

// ShadCN UI Components
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { Skeleton } from '@/components/ui/skeleton';
import { Button } from '@/components/ui/button';
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu';
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from '@/components/ui/alert-dialog';

interface AssetListProps {
  projectId: string;
}

export function AssetList({ projectId }: AssetListProps) {
  const [assets, setAssets] = useState<AssetItem[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  
  // Unified state for deleting any item (file or folder)
  const [itemToDelete, setItemToDelete] = useState<AssetItem | null>(null);
  const [isDeleting, setIsDeleting] = useState(false);

  const [isUploadDialogOpen, setIsUploadDialogOpen] = useState(false);
  const [isCreateFolderOpen, setIsCreateFolderOpen] = useState(false);
  const [currentPath, setCurrentPath] = useState<string[]>([]);

  const fetchAssets = useCallback(async () => {
    if (!projectId) return;
    setIsLoading(true);
    setError(null);
    const pathString = currentPath.join('/');
    try {
      const response = await api.listAssets(projectId, pathString);
      const data = response.data || [];
      const sortedAssets = data.sort((a, b) => {
        if (a.is_dir !== b.is_dir) return a.is_dir ? -1 : 1;
        return a.key.localeCompare(b.key);
      });
      setAssets(sortedAssets);
    } catch (err) {
      setError("Failed to load assets.");
      console.error(err);
    } finally {
      setIsLoading(false);
    }
  }, [projectId, currentPath]);

  useEffect(() => {
    fetchAssets();
  }, [fetchAssets]);

  const handleConfirmDelete = async () => {
    if (!itemToDelete) return;
    
    setIsDeleting(true);
    const fullPath = [...currentPath, itemToDelete.key].join('/');

    try {
      if (itemToDelete.is_dir) {
        // It's a folder, call the new deleteFolder endpoint
        await api.deleteFolder(projectId, fullPath);
      } else {
        // It's a file, call the existing deleteAsset endpoint
        await api.deleteAsset(projectId, fullPath);
      }
      setItemToDelete(null); // Close the dialog
      await fetchAssets(); // Refresh the list
    } catch (err) {
      console.error("Failed to delete item", err);
      setError(`Failed to delete ${itemToDelete.is_dir ? 'folder' : 'file'}.`);
    } finally {
      setIsDeleting(false);
    }
  };

  const handleFolderClick = (folderName: string) => {
    setCurrentPath(prev => [...prev, folderName]);
  };

  const handleBreadcrumbClick = (index: number) => {
    setCurrentPath(prev => prev.slice(0, index + 1));
  };

  return (
    <>
      <Card>
        <CardHeader className="flex flex-row items-start justify-between">
          <div>
            <CardTitle>Project Assets</CardTitle>
            <div className="flex items-center gap-1 text-sm text-muted-foreground mt-2">
              <Home className="h-4 w-4" />
              <Button variant="link" className="p-0 h-auto" onClick={() => setCurrentPath([])}>Root</Button>
              {currentPath.map((segment, index) => (
                <div key={index} className="flex items-center gap-1">
                  <ChevronRight className="h-4 w-4" />
                  <Button variant="link" className="p-0 h-auto" onClick={() => handleBreadcrumbClick(index)}>{segment}</Button>
                </div>
              ))}
            </div>
          </div>
          <div className="flex items-center gap-2">
            <Button variant="outline" onClick={() => setIsCreateFolderOpen(true)}>
              <FolderPlus className="mr-2 h-4 w-4" />
              New Folder
            </Button>
            <Button onClick={() => setIsUploadDialogOpen(true)}>
              <Upload className="mr-2 h-4 w-4" />
              Upload Asset
            </Button>
          </div>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="w-[50px]"></TableHead>
                <TableHead>Name</TableHead>
                <TableHead>Size</TableHead>
                <TableHead>Last Modified</TableHead>
                <TableHead className="text-right w-20">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {isLoading ? (
                Array.from({ length: 3 }).map((_, i) => (
                  <TableRow key={i}>
                    <TableCell colSpan={5}><Skeleton className="h-8 w-full" /></TableCell>
                  </TableRow>
                ))
              ) : error ? (
                <TableRow><TableCell colSpan={5} className="h-24 text-center text-red-500">{error}</TableCell></TableRow>
              ) : assets.length === 0 ? (
                <TableRow><TableCell colSpan={5} className="h-24 text-center">No assets found in this directory.</TableCell></TableRow>
              ) : (
                assets.map((asset) => (
                  <TableRow key={asset.key}>
                    <TableCell>{asset.is_dir ? <Folder className="h-5 w-5" /> : <File className="h-5 w-5" />}</TableCell>
                    <TableCell className="font-medium">
                      {asset.is_dir ? (
                        <button onClick={() => handleFolderClick(asset.key)} className="hover:underline text-left">
                          {asset.key}
                        </button>
                      ) : (
                        asset.key
                      )}
                    </TableCell>
                    <TableCell>{asset.is_dir ? '-' : `${(asset.size / 1024).toFixed(2)} KB`}</TableCell>
                    <TableCell>{asset.is_dir ? '-' : format(new Date(asset.last_modified), 'PP pp')}</TableCell>
                    <TableCell className="text-right">
                      <DropdownMenu>
                        <DropdownMenuTrigger asChild>
                          <Button variant="ghost" className="h-8 w-8 p-0"><MoreHorizontal className="h-4 w-4" /></Button>
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end">
                          <DropdownMenuItem onClick={() => setItemToDelete(asset)} className="text-red-600 focus:text-red-600">
                            <Trash2 className="mr-2 h-4 w-4" />
                            <span>Delete</span>
                          </DropdownMenuItem>
                        </DropdownMenuContent>
                      </DropdownMenu>
                    </TableCell>
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
        </CardContent>
      </Card>

      <AlertDialog open={!!itemToDelete} onOpenChange={(open) => !open && setItemToDelete(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Are you absolutely sure?</AlertDialogTitle>
            <AlertDialogDescription>
              This action cannot be undone. This will permanently delete the {itemToDelete?.is_dir ? 'folder' : 'file'} <span className="font-bold">{itemToDelete?.key}</span>
              {itemToDelete?.is_dir && ' and all of its contents'}.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={isDeleting}>Cancel</AlertDialogCancel>
            <AlertDialogAction onClick={handleConfirmDelete} disabled={isDeleting} className="bg-destructive hover:bg-destructive/90">
              {isDeleting ? 'Deleting...' : 'Continue'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <UploadAssetDialog
        projectId={projectId}
        currentPath={currentPath.join('/')}
        open={isUploadDialogOpen}
        onOpenChange={setIsUploadDialogOpen}
        onUploadSuccess={fetchAssets}
      />

      <CreateFolderDialog
        projectId={projectId}
        currentPath={currentPath.join('/')}
        open={isCreateFolderOpen}
        onOpenChange={setIsCreateFolderOpen}
        onCreateSuccess={fetchAssets}
      />
    </>
  );
}
