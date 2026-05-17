# One cover image per Issue, no per-Story images by default

An **Issue** carries exactly one generated cover image. **Stories** do not have their own images in the default Issue anatomy. A per-Publication toggle can opt in to per-Story images for Publications whose format demands it (heavily visual content, product roundups), but it's off by default.

The reasonable assumption looking at the data model is that every Story has an `image_url` — that assumption is wrong on purpose. We chose Issue-level cover only because (a) image generation is the most expensive step in the pipeline by an order of magnitude, and 5–7 generations per Issue × N Publications × M sends multiplies fast; (b) image-heavy emails trigger clipping in Gmail and load slowly on mobile, hurting deliverability and open-engagement; (c) the cover image is what shows in inbox previews and at the top of the email — it does the bulk of the visual lift while per-Story images add diminishing returns; (d) every per-Story image is another piece of generated content the owner has to review or accept, multiplying edit-time cost.

Owners who genuinely need per-Story images can flip the Publication-level toggle; we should not flip the default.
