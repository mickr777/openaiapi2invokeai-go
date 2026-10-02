package admin

// flux2KleinEditWorkflowJSON is based on the working FLUX.2 Klein image-edit
// graph used by the Python proxy, normalized to stable node IDs so Quick Setup
// can patch the selected model and sub-model references.
const flux2KleinEditWorkflowJSON = `{
  "id": "flux2-klein-image-edit",
  "nodes": {
    "model_loader": {
      "id": "model_loader",
      "type": "flux2_klein_model_loader",
      "is_intermediate": true,
      "use_cache": true,
      "model": {"key": "REPLACE_WITH_MODEL_KEY", "name": "your-flux2klein-model", "base": "flux2", "type": "main"},
      "max_seq_len": 512
    },
    "input_image": {
      "id": "input_image",
      "type": "image",
      "is_intermediate": true,
      "use_cache": true,
      "image": {"image_name": "REPLACE_WITH_INPUT_IMAGE"}
    },
    "kontext": {
      "id": "kontext",
      "type": "flux_kontext",
      "is_intermediate": true,
      "use_cache": true
    },
    "text_encoder": {
      "id": "text_encoder",
      "type": "flux2_klein_text_encoder",
      "is_intermediate": true,
      "use_cache": true,
      "prompt": "",
      "max_seq_len": 512
    },
    "denoise": {
      "id": "denoise",
      "type": "flux2_denoise",
      "is_intermediate": true,
      "use_cache": true,
      "denoising_start": 0.0,
      "denoising_end": 1.0,
      "add_noise": true,
      "guidance": 4.0,
      "cfg_scale": 1.0,
      "width": 1344,
      "height": 768,
      "num_steps": 9,
      "scheduler": "euler",
      "seed": 0
    },
    "decode": {
      "id": "decode",
      "type": "flux2_vae_decode",
      "is_intermediate": false,
      "use_cache": false
    }
  },
  "edges": [
    {"source": {"node_id": "model_loader", "field": "transformer"}, "destination": {"node_id": "denoise", "field": "transformer"}},
    {"source": {"node_id": "model_loader", "field": "vae"}, "destination": {"node_id": "denoise", "field": "vae"}},
    {"source": {"node_id": "model_loader", "field": "vae"}, "destination": {"node_id": "decode", "field": "vae"}},
    {"source": {"node_id": "model_loader", "field": "qwen3_encoder"}, "destination": {"node_id": "text_encoder", "field": "qwen3_encoder"}},
    {"source": {"node_id": "model_loader", "field": "max_seq_len"}, "destination": {"node_id": "text_encoder", "field": "max_seq_len"}},
    {"source": {"node_id": "input_image", "field": "image"}, "destination": {"node_id": "kontext", "field": "image"}},
    {"source": {"node_id": "input_image", "field": "width"}, "destination": {"node_id": "denoise", "field": "width"}},
    {"source": {"node_id": "input_image", "field": "height"}, "destination": {"node_id": "denoise", "field": "height"}},
    {"source": {"node_id": "kontext", "field": "kontext_cond"}, "destination": {"node_id": "denoise", "field": "kontext_conditioning"}},
    {"source": {"node_id": "text_encoder", "field": "conditioning"}, "destination": {"node_id": "denoise", "field": "positive_text_conditioning"}},
    {"source": {"node_id": "denoise", "field": "latents"}, "destination": {"node_id": "decode", "field": "latents"}}
  ]
}`
