// Filename: components/render/NewRenderDialog.tsx
"use client";

import { useState } from 'react';
import { toast } from "sonner";
import * as api from '@/lib/api';
import { parseTemplateForForm, FormField } from '@/lib/template-utils';
import { AssetSelector } from './AssetSelector';


import { Button } from '@/components/ui/button';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from '@/components/ui/dialog';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Label } from '@/components/ui/label';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import { Plus, Trash2, FolderSearch } from 'lucide-react';
import { Skeleton } from '@/components/ui/skeleton';
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group';

interface NewRenderDialogProps {
  projectId: string;
  templates: api.Template[];
}

type BackgroundMode = 'asset' | 'url';

export function NewRenderDialog({ projectId, templates }: NewRenderDialogProps) {
  const [open, setOpen] = useState(false);
  const [selectedTemplate, setSelectedTemplate] = useState<api.Template | null>(null);
  const [isFetchingTemplate, setIsFetchingTemplate] = useState(false);
  const [formFields, setFormFields] = useState<FormField[]>([]);
  const [payload, setPayload] = useState<Record<string, string | string[]>>({});
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [backgroundMode, setBackgroundMode] = useState<BackgroundMode>('asset');
  const [isAssetSelectorOpen, setIsAssetSelectorOpen] = useState(false);
  const [selectingAssetFor, setSelectingAssetFor] = useState<string | null>(null);

  const handleTemplateSelect = async (uuid: string) => {
    if (!uuid) return;
    const template = templates.find(t => t.uuid === uuid);
    setSelectedTemplate(template || null);
    setFormFields([]);
    setPayload({});
    setIsFetchingTemplate(true);
    try {
      const response = await api.getTemplateByUuid(uuid);
      const fields = parseTemplateForForm(response.data);
      setFormFields(fields);
      const initialPayload = fields.reduce((acc, field) => {
        acc[field.name] = field.type === 'carousel' ? [''] : '';
        return acc;
      }, {} as Record<string, string | string[]>);
      setPayload(initialPayload);
    } catch (error) {
      console.error("Failed to fetch template details", error);
      toast.error("Failed to load template details.");
    } finally {
      setIsFetchingTemplate(false);
    }
  };

  const handlePayloadChange = (field: string, value: string) => setPayload(prev => ({ ...prev, [field]: value }));
  const handleCarouselChange = (index: number, value: string) => {
    const newTexts = [...(payload.texts as string[])];
    newTexts[index] = value;
    setPayload(prev => ({ ...prev, texts: newTexts }));
  };
  const addCarouselItem = () => setPayload(prev => ({ ...prev, texts: [...(prev.texts as string[]), ''] }));
  const removeCarouselItem = (index: number) => {
    const newTexts = (payload.texts as string[]).filter((_, i) => i !== index);
    setPayload(prev => ({ ...prev, texts: newTexts }));
  };

  const handleAssetSelect = (assetPath: string) => {
    if (selectingAssetFor) handlePayloadChange(selectingAssetFor, assetPath);
    setSelectingAssetFor(null);
  };

  const openAssetSelector = (fieldName: string) => {
    setSelectingAssetFor(fieldName);
    setIsAssetSelectorOpen(true);
  };

  const handleSubmit = async () => {
    if (!selectedTemplate) return;
    setIsSubmitting(true);
    try {
      const finalPayload: Record<string, unknown> = { ...payload };
      let backgroundUrl: string | undefined = undefined;

      if (backgroundMode === 'url') {
        backgroundUrl = finalPayload.background as string;
        // The backend uses BackgroundImageURL, so we don't need the 'background' key in the payload
        delete finalPayload.background;
      }
      
      const response = await api.createRenderJob(projectId, {
        templateId: selectedTemplate.id,
        payload: finalPayload,
        backgroundImageUrl: backgroundUrl,
      });
      toast.success("Render job submitted successfully!", { description: `Job ID: ${response.data.jobId}` });
      setOpen(false);
    } catch (error) {
      console.error("Failed to submit job", error);
      toast.error("Failed to submit render job.");
    } finally {
      setIsSubmitting(false);
    }
  };

  const handleOpenChange = (isOpen: boolean) => {
    setOpen(isOpen);
    if (!isOpen) {
      setSelectedTemplate(null);
      setFormFields([]);
      setPayload({});
    }
  };

  const renderFormField = (field: FormField) => {
    switch (field.type) {
      case 'background':
        return (
          <div className="col-span-3 space-y-2">
            <ToggleGroup type="single" value={backgroundMode} onValueChange={(v: BackgroundMode) => v && setBackgroundMode(v)} className="w-full grid grid-cols-2">
              <ToggleGroupItem value="asset">From Assets</ToggleGroupItem>
              <ToggleGroupItem value="url">From URL</ToggleGroupItem>
            </ToggleGroup>
            {backgroundMode === 'asset' ? (
              <div className="flex gap-2">
                <Input id={field.name} value={payload[field.name] as string} readOnly placeholder="Select an asset..." />
                <Button variant="outline" onClick={() => openAssetSelector(field.name)}><FolderSearch className="h-4 w-4" /></Button>
              </div>
            ) : (
              <Input id={field.name} value={payload[field.name] as string} onChange={(e) => handlePayloadChange(field.name, e.target.value)} placeholder="https://example.com/image.png" />
            )}
          </div>
        );
      case 'asset':
        return (
          <div className="col-span-3 flex gap-2">
            <Input id={field.name} value={payload[field.name] as string} readOnly placeholder="Select an asset..." />
            <Button variant="outline" onClick={() => openAssetSelector(field.name)}><FolderSearch className="h-4 w-4" /></Button>
          </div>
        );
      case 'carousel':
        return (
          <div className="col-span-3 space-y-2">
            {(payload.texts as string[]).map((text, index) => (
              <div key={index} className="flex items-center gap-2">
                <Input value={text} onChange={(e) => handleCarouselChange(index, e.target.value)} />
                <Button variant="ghost" size="icon" onClick={() => removeCarouselItem(index)} disabled={(payload.texts as string[]).length <= 1}>
                  <Trash2 className="h-4 w-4" />
                </Button>
              </div>
            ))}
            <Button variant="outline" size="sm" onClick={addCarouselItem}>Add Item</Button>
          </div>
        );
      case 'textarea':
        return <Textarea id={field.name} value={payload[field.name] as string} onChange={(e) => handlePayloadChange(field.name, e.target.value)} className="col-span-3" rows={3} />;
      case 'text':
      default:
        return <Input id={field.name} value={payload[field.name] as string} onChange={(e) => handlePayloadChange(field.name, e.target.value)} className="col-span-3" />;
    }
  };

  return (
    <>
      <Dialog open={open} onOpenChange={handleOpenChange}>
        <DialogTrigger asChild><Button><Plus className="mr-2 h-4 w-4" />New Render</Button></DialogTrigger>
        <DialogContent className="sm:max-w-[600px]">
          <DialogHeader>
            <DialogTitle>Create a New Video</DialogTitle>
            <DialogDescription>Select a template and fill in the details.</DialogDescription>
          </DialogHeader>
          <div className="grid gap-6 py-4 max-h-[60vh] overflow-y-auto pr-4">
            <div className="grid grid-cols-4 items-center gap-4">
              <Label htmlFor="template" className="text-right">Template</Label>
              <Select onValueChange={handleTemplateSelect} value={selectedTemplate?.uuid || ''}>
                <SelectTrigger className="col-span-3"><SelectValue placeholder="Select a template..." /></SelectTrigger>
                <SelectContent>{templates.map(t => <SelectItem key={t.uuid} value={t.uuid}>{t.id} ({t.is_system ? 'System' : 'Custom'})</SelectItem>)}</SelectContent>
              </Select>
            </div>
            {isFetchingTemplate && <div className="space-y-4 col-span-4"><Skeleton className="h-24 w-full" /><Skeleton className="h-24 w-full" /></div>}
            {!isFetchingTemplate && formFields.map(field => (
              <div key={field.name} className="grid grid-cols-4 items-start gap-4">
                <Label htmlFor={field.name} className="text-right capitalize pt-2">{field.label}</Label>
                {renderFormField(field)}
              </div>
            ))}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setOpen(false)} disabled={isSubmitting}>Cancel</Button>
            <Button onClick={handleSubmit} disabled={isSubmitting || !selectedTemplate || formFields.length === 0}>{isSubmitting ? 'Submitting...' : 'Submit Job'}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <AssetSelector projectId={projectId} open={isAssetSelectorOpen} onOpenChange={setIsAssetSelectorOpen} onAssetSelect={handleAssetSelect} />
    </>
  );
}